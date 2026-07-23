package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	Email       EmailConfig
	AI          AIConfig
	Export      ExportConfig
}

type JWTConfig struct {
	Secret          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	// RedirectURL is the default used by the popup flow ("postmessage").
	RedirectURL string
	// AllowedRedirects are the exact redirect_uri values the redirect flow may
	// request. They must match what is registered in Google Cloud Console.
	AllowedRedirects []string
}

// EmailProvider selects the outbound mail transport.
type EmailProvider string

const (
	EmailProviderSMTP  EmailProvider = "smtp"
	EmailProviderBrevo EmailProvider = "brevo"
)

// EmailConfig covers both transports. Which set of fields matters depends on
// Provider, so validation is deferred to Validate rather than done by making
// every variable mandatory at startup.
type EmailConfig struct {
	Provider EmailProvider
	From     string
	FromName string
	Timeout  time.Duration

	SMTPHost     string
	SMTPPort     int
	SMTPUser     string
	SMTPPassword string

	BrevoAPIKey  string
	BrevoBaseURL string
}

// Validate reports the missing variables for the selected provider only.
func (c EmailConfig) Validate() error {
	if strings.TrimSpace(c.From) == "" {
		return errors.New("EMAIL_FROM (or SMTP_FROM) is required")
	}

	switch c.Provider {
	case EmailProviderBrevo:
		if strings.TrimSpace(c.BrevoAPIKey) == "" {
			return errors.New("BREVO_API_KEY is required when EMAIL_PROVIDER=brevo")
		}
	case EmailProviderSMTP:
		if strings.TrimSpace(c.SMTPHost) == "" {
			return errors.New("SMTP_HOST is required when EMAIL_PROVIDER=smtp")
		}
		if c.SMTPPort <= 0 || c.SMTPPort > 65535 {
			return fmt.Errorf("SMTP_PORT must be a valid port, got %d", c.SMTPPort)
		}
	default:
		return fmt.Errorf("unknown EMAIL_PROVIDER %q (expected smtp or brevo)", c.Provider)
	}

	return nil
}

// AIConfig drives the Gemini-backed summary and sprint-planning features.
// An empty APIKey disables those endpoints instead of failing startup.
type AIConfig struct {
	APIKey      string
	Model       string
	BaseURL     string
	Timeout     time.Duration
	MaxRetries  int
	Temperature float64
}

func (c AIConfig) Enabled() bool { return strings.TrimSpace(c.APIKey) != "" }

// ExportConfig governs outbound webhook pushes of analytics snapshots.
type ExportConfig struct {
	Timeout        time.Duration
	MaxRetries     int
	AllowPrivate   bool
	MaxPayloadSize int
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

	aiCfg := AIConfig{
		APIKey:      strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		Model:       getEnv("GEMINI_MODEL", "gemini-3.1-flash-lite"),
		BaseURL:     getEnv("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),
		Timeout:     getDuration("GEMINI_TIMEOUT", 90*time.Second, logger),
		MaxRetries:  getInt("GEMINI_MAX_RETRIES", 2, logger),
		Temperature: getFloat("GEMINI_TEMPERATURE", 0.3, logger),
	}
	if !aiCfg.Enabled() {
		logger.Warn("GEMINI_API_KEY is not set - AI summary and planning endpoints will return 503")
	}

	// The redirect flow hands the code back to the SPA origin, so that origin
	// is allowed by default; extra values cover other deployed front ends.
	frontendURL := mustGetEnv("FRONTEND_URL", logger)
	googleRedirects := appendUnique(
		[]string{strings.TrimRight(frontendURL, "/")},
		getCSV("GOOGLE_ALLOWED_REDIRECT_URLS")...,
	)

	emailCfg := loadEmailConfig(logger)
	if err := emailCfg.Validate(); err != nil {
		logger.Fatal("invalid email configuration", zap.Error(err))
	}
	logger.Info("email transport selected", zap.String("provider", string(emailCfg.Provider)))

	return &Config{
		Port:        mustGetEnv("PORT", logger),
		DatabaseURL: mustGetEnv("DATABASE_URL", logger),
		FrontendURL: frontendURL,
		JWT: JWTConfig{
			Secret:          mustGetEnv("JWT_SECRET", logger),
			AccessTokenTTL:  accessTTL,
			RefreshTokenTTL: refreshTTL,
		},
		Google: GoogleConfig{
			ClientID:         mustGetEnv("GOOGLE_CLIENT_ID", logger),
			ClientSecret:     mustGetEnv("GOOGLE_CLIENT_SECRET", logger),
			RedirectURL:      getEnv("GOOGLE_REDIRECT_URL", "postmessage"),
			AllowedRedirects: googleRedirects,
		},
		Email: emailCfg,
		AI:    aiCfg,
		Export: ExportConfig{
			Timeout:        getDuration("EXPORT_TIMEOUT", 15*time.Second, logger),
			MaxRetries:     getInt("EXPORT_MAX_RETRIES", 2, logger),
			AllowPrivate:   getBool("EXPORT_ALLOW_PRIVATE_TARGETS", false),
			MaxPayloadSize: getInt("EXPORT_MAX_PAYLOAD_BYTES", 8*1024*1024, logger),
		},
	}
}

// loadEmailConfig reads both transports' settings. The provider defaults to
// brevo whenever BREVO_API_KEY is present, so a deploy only has to set the key
// rather than remember a second switch — hosts that block outbound SMTP are
// exactly the ones where the key gets added.
func loadEmailConfig(logger *zap.Logger) EmailConfig {
	provider := EmailProvider(strings.ToLower(getEnv("EMAIL_PROVIDER", "")))
	if provider == "" {
		if strings.TrimSpace(os.Getenv("BREVO_API_KEY")) != "" {
			provider = EmailProviderBrevo
		} else {
			provider = EmailProviderSMTP
		}
	}

	// SMTP_FROM stays supported so existing deployments keep working.
	from := getEnv("EMAIL_FROM", getEnv("SMTP_FROM", ""))

	return EmailConfig{
		Provider: provider,
		From:     from,
		FromName: getEnv("EMAIL_FROM_NAME", "TaskFlow"),
		Timeout:  getDuration("EMAIL_TIMEOUT", 15*time.Second, logger),

		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     getInt("SMTP_PORT", 587, logger),
		SMTPUser:     getEnv("SMTP_USER", ""),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),

		BrevoAPIKey:  strings.TrimSpace(os.Getenv("BREVO_API_KEY")),
		BrevoBaseURL: getEnv("BREVO_BASE_URL", ""),
	}
}

// getCSV reads a comma-separated env var into trimmed, non-empty parts.
func getCSV(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimRight(strings.TrimSpace(part), "/"); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// appendUnique adds values that are not already present, preserving order.
func appendUnique(base []string, extra ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, v := range append(base, extra...) {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func mustGetEnv(key string, logger *zap.Logger) string {
	val := os.Getenv(key)
	if val == "" {
		logger.Fatal("Environment variable is required but not set", zap.String("variable", key))
	}
	return val
}

func getEnv(key, fallback string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return fallback
}

func getInt(key string, fallback int, logger *zap.Logger) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		logger.Warn("invalid integer env var, using default",
			zap.String("variable", key), zap.String("value", raw), zap.Int("default", fallback))
		return fallback
	}
	return val
}

func getFloat(key string, fallback float64, logger *zap.Logger) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	val, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		logger.Warn("invalid float env var, using default",
			zap.String("variable", key), zap.String("value", raw), zap.Float64("default", fallback))
		return fallback
	}
	return val
}

func getDuration(key string, fallback time.Duration, logger *zap.Logger) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	val, err := time.ParseDuration(raw)
	if err != nil {
		logger.Warn("invalid duration env var, using default",
			zap.String("variable", key), zap.String("value", raw), zap.Duration("default", fallback))
		return fallback
	}
	return val
}

func getBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	val, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return val
}
