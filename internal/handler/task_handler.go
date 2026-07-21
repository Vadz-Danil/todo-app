package handler

import (
	"errors"
	"net/http"

	"todo-app/internal/apperrors"
	"todo-app/internal/middleware"
	"todo-app/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TaskHandler struct {
	taskService  *service.TaskService
	emailService *service.EmailService
}

func NewTaskHandler(taskService *service.TaskService, emailService *service.EmailService) *TaskHandler {
	return &TaskHandler{
		taskService:  taskService,
		emailService: emailService,
	}
}

func (h *TaskHandler) CreateTask(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	var req CreateTaskRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.ErrEmptyTaskTitle.Error()})
		return
	}

	task, err := h.taskService.CreateTask(c.Request.Context(), userID, req.Title, req.Description)
	if err != nil {
		if errors.Is(err, apperrors.ErrEmptyTaskTitle) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}

	c.JSON(http.StatusCreated, task)
}

func (h *TaskHandler) GetTasks(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	tasks, err := h.taskService.GetTasks(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (h *TaskHandler) UpdateTaskStatus(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Task ID is required"})
		return
	}

	var req UpdateTaskStatusRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.ErrInvalidTaskStatus.Error()})
		return
	}

	err := h.taskService.UpdateTaskStatus(c.Request.Context(), taskID, userID, req.Status)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrInvalidTaskStatus):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, apperrors.ErrTaskNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task status"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task status updated successfully"})
}

func (h *TaskHandler) ShareTasks(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	var req ShareTasksRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.ErrEmptyRecipient.Error()})
		return
	}

	tasks, err := h.taskService.GetTasks(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve tasks for sharing"})
		return
	}

	taskTitles := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskTitles = append(taskTitles, task.Title)
	}

	err = h.emailService.ShareTasks(c.Request.Context(), req.RecipientEmail, taskTitles)
	if err != nil {
		switch {
		case errors.Is(err, apperrors.ErrEmptyRecipient), errors.Is(err, apperrors.ErrEmptyTaskList):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send email"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task list shared successfully"})
}

func getUserIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	userIDVal, exists := c.Get(middleware.UserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": apperrors.ErrUnauthorized.Error()})
		return uuid.Nil, false
	}

	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": apperrors.ErrUnauthorized.Error()})
		return uuid.Nil, false
	}

	return userID, true
}
