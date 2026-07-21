package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
	ErrTokenExpired = errors.New("token has expired")
	ErrWrongType    = errors.New("invalid token type")
)

type CustomClaims struct {
	UserID    string `json:"user_id"`
	TokenType string `json:"token_type"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	secret     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewTokenManager(secret string, accessTTL, refreshTTL time.Duration) (*TokenManager, error) {
	if secret == "" {
		return nil, errors.New("jwt secret cannot be empty")
	}
	return &TokenManager{
		secret:     secret,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}, nil
}

func (m *TokenManager) GenerateTokenPair(userID uuid.UUID) (accessToken string, refreshToken string, err error) {
	accessToken, err = m.generate(userID, "access", m.accessTTL)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, err = m.generate(userID, "refresh", m.refreshTTL)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

func (m *TokenManager) ParseAccessToken(tokenStr string) (uuid.UUID, error) {
	return m.parse(tokenStr, "access")
}

func (m *TokenManager) ParseRefreshToken(tokenStr string) (uuid.UUID, error) {
	return m.parse(tokenStr, "refresh")
}

func (m *TokenManager) generate(userID uuid.UUID, tokenType string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := CustomClaims{
		UserID:    userID.String(),
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.secret))
}

func (m *TokenManager) parse(tokenStr string, expectedType string) (uuid.UUID, error) {
	claims := &CustomClaims{}

	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(m.secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return uuid.Nil, ErrTokenExpired
		}
		return uuid.Nil, ErrInvalidToken
	}

	if !token.Valid {
		return uuid.Nil, ErrInvalidToken
	}

	if claims.TokenType != expectedType {
		return uuid.Nil, ErrWrongType
	}

	parsedUUID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid user_id in token: %w", err)
	}

	return parsedUUID, nil
}
