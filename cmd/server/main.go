package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"todo-app/internal/config"
	"todo-app/internal/handler"
	"todo-app/internal/provider"
	"todo-app/internal/repository"
	"todo-app/internal/routers"
	"todo-app/internal/service"
	"todo-app/pkg/email"
	"todo-app/pkg/gemini"
	"todo-app/pkg/jwt"
	"todo-app/pkg/logger"

	ginzap "github.com/gin-contrib/zap"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func main() {
	zapLogger, err := logger.InitLogger("dev")
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer func() {
		if err := zapLogger.Sync(); err != nil {
			_ = err
		}
	}()

	cfg := config.LoadConfig(zapLogger)

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		zapLogger.Fatal("failed to open database connection", zap.Error(err))
	}

	defer func() {
		if err := db.Close(); err != nil {
			zapLogger.Error("failed to close database connection", zap.Error(err))
		}
	}()

	if err := db.Ping(); err != nil {
		zapLogger.Fatal("failed to ping database", zap.Error(err))
	}
	zapLogger.Info("successfully connected to database")

	tokenManager, err := jwt.NewTokenManager(
		cfg.JWT.Secret,
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
	)
	if err != nil {
		zapLogger.Fatal("failed to initialize token manager", zap.Error(err))
	}

	googleProvider := provider.NewGoogleProvider(
		cfg.Google.ClientID,
		cfg.Google.ClientSecret,
		cfg.Google.RedirectURL,
		cfg.Google.AllowedRedirects,
	)

	mailer := newMailSender(cfg.Email)

	userRepo := repository.NewUserPostgres(db)
	taskRepo := repository.NewTaskPostgres(db)
	analyticsRepo := repository.NewAnalyticsPostgres(db)
	sprintRepo := repository.NewSprintPostgres(db)
	planningRepo := repository.NewPlanningPostgres(db)
	summaryRepo := repository.NewSummaryPostgres(db)
	exportRepo := repository.NewExportPostgres(db)

	// A nil interface (not a typed nil) is what disables the AI features.
	var geminiClient gemini.Client
	if cfg.AI.Enabled() {
		geminiClient = gemini.New(gemini.Config{
			APIKey:      cfg.AI.APIKey,
			Model:       cfg.AI.Model,
			BaseURL:     cfg.AI.BaseURL,
			Timeout:     cfg.AI.Timeout,
			MaxRetries:  cfg.AI.MaxRetries,
			Temperature: cfg.AI.Temperature,
		})
		zapLogger.Info("AI features enabled", zap.String("model", cfg.AI.Model))
	}

	authService := service.NewAuthService(userRepo, tokenManager, googleProvider, zapLogger)
	taskService := service.NewTaskService(taskRepo, sprintRepo, zapLogger)
	analyticsService := service.NewAnalyticsService(analyticsRepo, zapLogger)
	sprintService := service.NewSprintService(sprintRepo, taskRepo, zapLogger)
	aiService := service.NewAIService(geminiClient, planningRepo, summaryRepo, sprintRepo, analyticsService, taskService, zapLogger)
	exportService := service.NewExportService(exportRepo, analyticsService, taskService, sprintService, cfg.Export, zapLogger)
	emailService, err := service.NewEmailService(mailer, zapLogger)
	if err != nil {
		zapLogger.Fatal("failed to initialize email service", zap.Error(err))
	}

	r := gin.New()
	r.Use(ginzap.Ginzap(zapLogger, time.RFC3339, true))
	r.Use(ginzap.RecoveryWithZap(zapLogger, true))

	routers.SetupRoutes(r, tokenManager, routers.Handlers{
		Auth:      handler.NewAuthHandler(authService),
		Task:      handler.NewTaskHandler(taskService, emailService, authService, cfg.FrontendURL),
		Analytics: handler.NewAnalyticsHandler(analyticsService),
		AI:        handler.NewAIHandler(aiService, analyticsService, authService),
		Sprint:    handler.NewSprintHandler(sprintService),
		Export:    handler.NewExportHandler(exportService, analyticsService, authService),
	})

	// AI generation is the longest thing this server does: a Gemini call can take
	// GEMINI_TIMEOUT (90s by default) and is retried, so a 10s write timeout would
	// cut the response off after the work had already been committed. Header reads
	// stay short so a slow-loris client still cannot hold a connection open.
	writeTimeout := 30 * time.Second
	if cfg.AI.Enabled() {
		writeTimeout = cfg.AI.Timeout*time.Duration(cfg.AI.MaxRetries+1) + 30*time.Second
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		zapLogger.Info("starting HTTP server", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			zapLogger.Fatal("HTTP server failed to run", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("shutting down HTTP server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		zapLogger.Error("server forced to shutdown", zap.Error(err))
	}

	zapLogger.Info("server exited cleanly")
}

// newMailSender picks the outbound mail transport. Config.Validate has already
// checked that the selected provider has what it needs.
func newMailSender(cfg config.EmailConfig) email.Sender {
	switch cfg.Provider {
	case config.EmailProviderBrevo:
		return email.NewBrevoSender(email.BrevoConfig{
			APIKey:   cfg.BrevoAPIKey,
			BaseURL:  cfg.BrevoBaseURL,
			From:     cfg.From,
			FromName: cfg.FromName,
			Timeout:  cfg.Timeout,
		})

	case config.EmailProviderMailjet:
		return email.NewMailjetSender(email.MailjetConfig{
			APIKey:    cfg.MailjetAPIKey,
			SecretKey: cfg.MailjetSecretKey,
			BaseURL:   cfg.MailjetBaseURL,
			From:      cfg.From,
			FromName:  cfg.FromName,
			Timeout:   cfg.Timeout,
		})

	case config.EmailProviderSMTP2GO:
		return email.NewSMTP2GOSender(email.SMTP2GOConfig{
			APIKey:   cfg.SMTP2GOAPIKey,
			BaseURL:  cfg.SMTP2GOBaseURL,
			From:     cfg.From,
			FromName: cfg.FromName,
			Timeout:  cfg.Timeout,
		})
	}

	return email.NewSMTPSender(email.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUser,
		Password: cfg.SMTPPassword,
		From:     cfg.From,
		FromName: cfg.FromName,
		Timeout:  cfg.Timeout,
	})
}
