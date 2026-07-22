package config

import (
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

type Config struct {
	Port        string
	DatabaseURL string
	FrontendURL string
	JWT         JWTConfig
	Google      GoogleConfig
	SMTP        SMTPConfig
}

type JWTConfig struct {
	Secret          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type SMTPConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	From     string
}

func LoadConfig(logger *zap.Logger) *Config {
	if err := godotenv.Load(); err != nil {
		logger.Info("Note: .env file not found, loading directly from system environment")
	}

	accessTTLStr := mustGetEnv("JWT_ACCESS_TOKEN_TTL", logger)
	accessTTL, err := time.ParseDuration(accessTTLStr)
	if err != nil {
		logger.Fatal("Invalid JWT_ACCESS_TOKEN_TTL duration format", zap.Error(err))
	}

	refreshTTLStr := mustGetEnv("JWT_REFRESH_TOKEN_TTL", logger)
	refreshTTL, err := time.ParseDuration(refreshTTLStr)
	if err != nil {
		logger.Fatal("Invalid JWT_REFRESH_TOKEN_TTL duration format", zap.Error(err))
	}

	return &Config{
		Port:        mustGetEnv("PORT", logger),
		DatabaseURL: mustGetEnv("DATABASE_URL", logger),
		FrontendURL: mustGetEnv("FRONTEND_URL", logger),
		JWT: JWTConfig{
			Secret:          mustGetEnv("JWT_SECRET", logger),
			AccessTokenTTL:  accessTTL,
			RefreshTokenTTL: refreshTTL,
		},
		Google: GoogleConfig{
			ClientID:     mustGetEnv("GOOGLE_CLIENT_ID", logger),
			ClientSecret: mustGetEnv("GOOGLE_CLIENT_SECRET", logger),
			RedirectURL:  mustGetEnv("GOOGLE_REDIRECT_URL", logger),
		},
		SMTP: SMTPConfig{
			Host:     mustGetEnv("SMTP_HOST", logger),
			Port:     mustGetEnv("SMTP_PORT", logger),
			User:     mustGetEnv("SMTP_USER", logger),
			Password: mustGetEnv("SMTP_PASSWORD", logger),
			From:     mustGetEnv("SMTP_FROM", logger),
		},
	}
}

func mustGetEnv(key string, logger *zap.Logger) string {
	val := os.Getenv(key)
	if val == "" {
		logger.Fatal("Environment variable is required but not set", zap.String("variable", key))
	}
	return val
}
