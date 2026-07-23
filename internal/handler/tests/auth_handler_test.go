package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"todo-app/internal/apperrors"
	"todo-app/internal/handler"
	"todo-app/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockAuthService struct {
	mock.Mock
}

func (m *MockAuthService) Register(ctx context.Context, email, password string) error {
	args := m.Called(ctx, email, password)
	return args.Error(0)
}

func (m *MockAuthService) Login(ctx context.Context, email, password string) (string, string, error) {
	args := m.Called(ctx, email, password)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockAuthService) GoogleLogin(ctx context.Context, code, redirectURI string) (string, string, error) {
	args := m.Called(ctx, code, redirectURI)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockAuthService) RefreshToken(refreshTokenStr string) (string, string, error) {
	args := m.Called(refreshTokenStr)
	return args.String(0), args.String(1), args.Error(2)
}

func (m *MockAuthService) GetUserByID(ctx context.Context, userID uuid.UUID) (*models.User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.User), args.Error(1)
}

func setupRouter(authService *MockAuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	authH := handler.NewAuthHandler(authService)
	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", authH.Register)
		authGroup.POST("/login", authH.Login)
		authGroup.POST("/google", authH.GoogleLogin)
		authGroup.POST("/refresh", authH.RefreshToken)
	}

	return r
}

func TestAuthHandler_Register(t *testing.T) {
	t.Run("Success - 201 Created", func(t *testing.T) {
		mockAuth := new(MockAuthService)
		mockAuth.On("Register", mock.Anything, "test@example.com", "password123").Return(nil)

		r := setupRouter(mockAuth)

		body, _ := json.Marshal(map[string]string{
			"email":    "test@example.com",
			"password": "password123",
		})

		req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		mockAuth.AssertExpectations(t)
	})

	t.Run("User Already Exists - 409 Conflict", func(t *testing.T) {
		mockAuth := new(MockAuthService)
		mockAuth.On("Register", mock.Anything, "test@example.com", "password123").Return(apperrors.ErrUserAlreadyExists)

		r := setupRouter(mockAuth)

		body, _ := json.Marshal(map[string]string{
			"email":    "test@example.com",
			"password": "password123",
		})

		req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
		mockAuth.AssertExpectations(t)
	})

	t.Run("Invalid Payload - 400 Bad Request", func(t *testing.T) {
		mockAuth := new(MockAuthService)
		r := setupRouter(mockAuth)

		body := []byte(`{"email": "test@example.com"}`)

		req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestAuthHandler_Login(t *testing.T) {
	t.Run("Success - 200 OK", func(t *testing.T) {
		mockAuth := new(MockAuthService)
		mockAuth.On("Login", mock.Anything, "test@example.com", "password123").
			Return("access_token_abc", "refresh_token_xyz", nil)

		r := setupRouter(mockAuth)

		body, _ := json.Marshal(map[string]string{
			"email":    "test@example.com",
			"password": "password123",
		})

		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "access_token_abc")
		mockAuth.AssertExpectations(t)
	})

	t.Run("Invalid Credentials - 401 Unauthorized", func(t *testing.T) {
		mockAuth := new(MockAuthService)
		mockAuth.On("Login", mock.Anything, "test@example.com", "wrongpass").
			Return("", "", apperrors.ErrInvalidCredentials)

		r := setupRouter(mockAuth)

		body, _ := json.Marshal(map[string]string{
			"email":    "test@example.com",
			"password": "wrongpass",
		})

		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		mockAuth.AssertExpectations(t)
	})
}

func TestAuthHandler_RefreshToken(t *testing.T) {
	t.Run("Success - 200 OK", func(t *testing.T) {
		mockAuth := new(MockAuthService)
		mockAuth.On("RefreshToken", "valid_refresh_token").
			Return("new_access_token", "new_refresh_token", nil)

		r := setupRouter(mockAuth)

		body, _ := json.Marshal(map[string]string{
			"refresh_token": "valid_refresh_token",
		})

		req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "new_access_token")
		mockAuth.AssertExpectations(t)
	})
}
