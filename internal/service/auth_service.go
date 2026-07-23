package service

import (
	"context"
	"errors"
	"strings"
	"todo-app/internal/provider"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/repository"
	"todo-app/pkg/jwt"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

type Auth interface {
	Register(ctx context.Context, email, password string) error
	Login(ctx context.Context, email, password string) (string, string, error)
	GoogleLogin(ctx context.Context, code, redirectURI string) (string, string, error)
	RefreshToken(refreshTokenStr string) (string, string, error)
	GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error)
}

type AuthService struct {
	userRepo       repository.UserRepository
	tokenManager   *jwt.TokenManager
	googleProvider *provider.GoogleProvider
	logger         *zap.Logger
}

func NewAuthService(
	userRepo repository.UserRepository,
	tokenManager *jwt.TokenManager,
	googleProvider *provider.GoogleProvider,
	logger *zap.Logger,
) *AuthService {
	return &AuthService{
		userRepo:       userRepo,
		tokenManager:   tokenManager,
		googleProvider: googleProvider,
		logger:         logger,
	}
}

func (s *AuthService) Register(ctx context.Context, email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(password) < 6 {
		return apperrors.ErrInvalidCredentials
	}

	existingUser, err := s.userRepo.GetUserByEmail(ctx, email)
	if err == nil {
		if existingUser.PasswordHash != nil {
			return apperrors.ErrUserAlreadyExists
		}
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			s.logger.Error("failed to hash password", zap.Error(err))
			return err
		}

		if err := s.userRepo.SetPasswordHash(ctx, existingUser.ID, string(hashedBytes)); err != nil {
			s.logger.Error("failed to link password to existing google user", zap.Error(err))
			return err
		}

		return nil
	} else if !errors.Is(err, apperrors.ErrUserNotFound) {
		s.logger.Error("failed to check existing user in db", zap.Error(err))
		return err
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		s.logger.Error("failed to hash password", zap.Error(err))
		return err
	}

	hashedPasswordStr := string(hashedBytes)
	user := &models.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: &hashedPasswordStr,
	}

	if err := s.userRepo.CreateUser(ctx, user); err != nil {
		if !errors.Is(err, apperrors.ErrUserAlreadyExists) {
			s.logger.Error("failed to create user in db", zap.Error(err))
		}
		return err
	}

	return nil
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, string, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	user, err := s.userRepo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, apperrors.ErrUserNotFound) {
			return "", "", apperrors.ErrInvalidCredentials
		}
		s.logger.Error("failed to get user by email", zap.Error(err))
		return "", "", err
	}

	if user.PasswordHash == nil {
		return "", "", apperrors.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(password)); err != nil {
		return "", "", apperrors.ErrInvalidCredentials
	}

	accessToken, refreshToken, err := s.tokenManager.GenerateTokenPair(user.ID)
	if err != nil {
		s.logger.Error("failed to generate token pair", zap.Error(err))
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func (s *AuthService) GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	user, err := s.userRepo.GetUserByID(ctx, userID)
	if err != nil {
		if !errors.Is(err, apperrors.ErrUserNotFound) {
			s.logger.Error("failed to get user by id", zap.Error(err), zap.String("user_id", userID.String()))
		}
		return nil, err
	}
	return user, nil
}

func (s *AuthService) GoogleLogin(ctx context.Context, code, redirectURI string) (string, string, error) {
	if strings.TrimSpace(code) == "" {
		return "", "", apperrors.ErrInvalidCredentials
	}

	googleUser, err := s.googleProvider.ExchangeCode(ctx, code, redirectURI)
	if err != nil {
		if errors.Is(err, provider.ErrRedirectURINotAllowed) {
			s.logger.Warn("rejected google login with an unregistered redirect_uri",
				zap.String("redirect_uri", redirectURI))
			return "", "", apperrors.ErrInvalidRedirectURI
		}
		s.logger.Error("failed to exchange google code", zap.Error(err))
		return "", "", apperrors.ErrInvalidCredentials
	}

	if !googleUser.VerifiedEmail {
		return "", "", apperrors.ErrInvalidCredentials
	}

	user, err := s.userRepo.FindOrCreateGoogleUser(ctx, googleUser.Email, googleUser.ID)
	if err != nil {
		s.logger.Error("failed to find or create google user in db", zap.Error(err))
		return "", "", err
	}

	accessToken, refreshToken, err := s.tokenManager.GenerateTokenPair(user.ID)
	if err != nil {
		s.logger.Error("failed to generate tokens for google user", zap.Error(err))
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

func (s *AuthService) RefreshToken(refreshTokenStr string) (string, string, error) {
	userID, err := s.tokenManager.ParseRefreshToken(refreshTokenStr)
	if err != nil {
		return "", "", apperrors.ErrUnauthorized
	}

	accessToken, refreshToken, err := s.tokenManager.GenerateTokenPair(userID)
	if err != nil {
		s.logger.Error("failed to generate token pair during refresh", zap.Error(err))
		return "", "", err
	}

	return accessToken, refreshToken, nil
}
