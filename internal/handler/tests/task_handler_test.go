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
	"todo-app/internal/middleware"
	"todo-app/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockTaskService struct {
	mock.Mock
}

func (m *MockTaskService) CreateTask(ctx context.Context, userID uuid.UUID, title string, description *string) (*models.Task, error) {
	args := m.Called(ctx, userID, title, description)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Task), args.Error(1)
}

func (m *MockTaskService) GetTasks(ctx context.Context, userID uuid.UUID) ([]models.Task, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Task), args.Error(1)
}

func (m *MockTaskService) UpdateTaskStatus(ctx context.Context, taskID string, userID uuid.UUID, status models.TaskStatus) error {
	args := m.Called(ctx, taskID, userID, status)
	return args.Error(0)
}

type MockEmailService struct {
	mock.Mock
}

func (m *MockEmailService) ShareTasks(ctx context.Context, recipientEmail, senderEmail, dashboardURL string) error {
	args := m.Called(ctx, recipientEmail, senderEmail, dashboardURL)
	return args.Error(0)
}

func setupTaskRouter(taskService *MockTaskService, emailService *MockEmailService, authService *MockAuthService, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	if userID != uuid.Nil {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.UserIDKey, userID)
			c.Next()
		})
	}
	frontendURL := "http://localhost:3000"
	h := handler.NewTaskHandler(taskService, emailService, authService, frontendURL)

	r.POST("/tasks", h.CreateTask)
	r.GET("/tasks", h.GetTasks)
	r.PATCH("/tasks/:id/status", h.UpdateTaskStatus)
	r.POST("/tasks/share", h.ShareTasks)

	return r
}

func TestTaskHandler_CreateTask(t *testing.T) {
	userID := uuid.New()

	t.Run("Success - 201 Created", func(t *testing.T) {
		mockTask := new(MockTaskService)
		expectedTask := &models.Task{
			ID:          uuid.New(),
			UserID:      userID,
			Title:       "New Task",
			Description: new("Task Description"),
		}

		mockTask.On("CreateTask", mock.Anything, userID, "New Task", mock.MatchedBy(func(desc *string) bool {
			return desc != nil && *desc == "Task Description"
		})).Return(expectedTask, nil)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		body, _ := json.Marshal(map[string]string{
			"title":       "New Task",
			"description": "Task Description",
		})

		req := httptest.NewRequest(http.MethodPost, "/tasks", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), "New Task")
		mockTask.AssertExpectations(t)
	})

	t.Run("Unauthorized - 401 Missing Context UserID", func(t *testing.T) {
		mockTask := new(MockTaskService)
		r := setupTaskRouter(mockTask, nil, nil, uuid.Nil)

		req := httptest.NewRequest(http.MethodPost, "/tasks", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestTaskHandler_UpdateTaskStatus(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New().String()

	t.Run("Success - 200 OK", func(t *testing.T) {
		mockTask := new(MockTaskService)
		mockTask.On("UpdateTaskStatus", mock.Anything, taskID, userID, models.TaskStatus("DONE")).Return(nil)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		body, _ := json.Marshal(map[string]string{
			"status": "DONE",
		})

		req := httptest.NewRequest(http.MethodPatch, "/tasks/"+taskID+"/status", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		mockTask.AssertExpectations(t)
	})

	t.Run("Invalid Task UUID Format - 400 Bad Request", func(t *testing.T) {
		mockTask := new(MockTaskService)
		r := setupTaskRouter(mockTask, nil, nil, userID)

		body, _ := json.Marshal(map[string]string{"status": "DONE"})

		req := httptest.NewRequest(http.MethodPatch, "/tasks/invalid-uuid-format/status", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid task ID format")
	})

	t.Run("Task Not Found - 404 Not Found", func(t *testing.T) {
		mockTask := new(MockTaskService)
		mockTask.On("UpdateTaskStatus", mock.Anything, taskID, userID, models.TaskStatus("DONE")).Return(apperrors.ErrTaskNotFound)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		body, _ := json.Marshal(map[string]string{"status": "DONE"})

		req := httptest.NewRequest(http.MethodPatch, "/tasks/"+taskID+"/status", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		mockTask.AssertExpectations(t)
	})
}

func TestTaskHandler_ShareTasks(t *testing.T) {
	userID := uuid.New()
	senderEmail := "sender@example.com"
	recipientEmail := "friend@example.com"

	t.Run("Success - 200 OK", func(t *testing.T) {
		mockAuth := new(MockAuthService)
		mockEmail := new(MockEmailService)

		user := &models.User{ID: userID, Email: senderEmail}

		mockAuth.On("GetUserByID", mock.Anything, userID).Return(user, nil)
		mockEmail.On("ShareTasks", mock.Anything, recipientEmail, senderEmail, "http://localhost:3000/dashboard").Return(nil)

		r := setupTaskRouter(nil, mockEmail, mockAuth, userID)

		body, _ := json.Marshal(map[string]string{
			"recipient_email": recipientEmail,
		})

		req := httptest.NewRequest(http.MethodPost, "/tasks/share", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		mockAuth.AssertExpectations(t)
		mockEmail.AssertExpectations(t)
	})
}
