package main

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"todo-app/internal/config"
	"todo-app/internal/handler"
	"todo-app/internal/provider"
	"todo-app/internal/repository"
	"todo-app/internal/routers"
	"todo-app/internal/service"
	"todo-app/pkg/email"
	"todo-app/pkg/jwt"
	"todo-app/pkg/logger"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

func main() {
	zapLogger, err := logger.InitLogger("dev")
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer zapLogger.Sync()

	cfg := config.LoadConfig(zapLogger)

	db, err := sql.Open("postgres", cfg.DatabaseURL)
	if err != nil {
		zapLogger.Fatal("failed to open database connection", zap.Error(err))
	}
	defer db.Close()

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
	)

	smtpPort, err := strconv.Atoi(cfg.SMTP.Port)
	if err != nil {
		zapLogger.Fatal("invalid SMTP port in config", zap.Error(err))
	}

	mailer := email.NewMailer(email.Config{
		Host:     cfg.SMTP.Host,
		Port:     smtpPort,
		Username: cfg.SMTP.User,
		Password: cfg.SMTP.Password,
		From:     cfg.SMTP.From,
	})

	userRepo := repository.NewUserPostgres(db)
	taskRepo := repository.NewTaskPostgres(db)

	authService := service.NewAuthService(userRepo, tokenManager, googleProvider, zapLogger)
	taskService := service.NewTaskService(taskRepo, zapLogger)
	emailService := service.NewEmailService(mailer, zapLogger)

	authHandler := handler.NewAuthHandler(authService)
	taskHandler := handler.NewTaskHandler(taskService, emailService)

	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	routers.SetupRoutes(r, tokenManager, authHandler, taskHandler)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
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
