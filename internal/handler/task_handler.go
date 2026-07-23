package handler

import (
	"fmt"
	"net/http"
	"strings"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type TaskHandler struct {
	taskService  service.Task
	emailService service.Email
	authService  service.Auth
	frontendURL  string
}

func NewTaskHandler(taskService service.Task, emailService service.Email, authService service.Auth, frontendURL string) *TaskHandler {
	return &TaskHandler{
		taskService:  taskService,
		emailService: emailService,
		authService:  authService,
		frontendURL:  frontendURL,
	}
}

func (h *TaskHandler) CreateTask(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task payload: a JSON object with a non-empty title is required"})
		return
	}

	in, err := req.toCreate()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.taskService.CreateTask(c.Request.Context(), userID, in)
	if respondServiceError(c, err, "Failed to create task") {
		return
	}

	c.JSON(http.StatusCreated, task)
}

func (h *TaskHandler) GetTasks(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	filter, ok := parseTaskFilter(c)
	if !ok {
		return
	}

	tasks, err := h.taskService.GetTasks(c.Request.Context(), userID, filter)
	if respondServiceError(c, err, "Failed to fetch tasks") {
		return
	}
	if tasks == nil {
		tasks = []models.Task{}
	}

	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

func (h *TaskHandler) GetTask(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	taskID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	task, err := h.taskService.GetTask(c.Request.Context(), userID, taskID)
	if respondServiceError(c, err, "Failed to fetch task") {
		return
	}

	c.JSON(http.StatusOK, task)
}

func (h *TaskHandler) UpdateTask(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	taskID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task payload: expected a JSON object of fields to update"})
		return
	}

	patch, err := req.toPatch()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	task, err := h.taskService.UpdateTask(c.Request.Context(), userID, taskID, patch)
	if respondServiceError(c, err, "Failed to update task") {
		return
	}

	c.JSON(http.StatusOK, task)
}

func (h *TaskHandler) UpdateTaskStatus(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	taskID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	var req UpdateTaskStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status payload: a non-empty status is required"})
		return
	}

	ctx := c.Request.Context()

	err := h.taskService.UpdateTaskStatus(ctx, taskID.String(), userID, req.Status, req.Reviewer)
	if respondServiceError(c, err, "Failed to update task status") {
		return
	}

	task, err := h.taskService.GetTask(ctx, userID, taskID)
	if respondServiceError(c, err, "Failed to fetch updated task") {
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task status updated successfully", "task": task})
}

func (h *TaskHandler) MoveTask(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	taskID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	var req MoveTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid move payload: a target status and optional UUID neighbours are required"})
		return
	}

	task, err := h.taskService.MoveTask(c.Request.Context(), userID, taskID, req.Status, req.AfterID, req.BeforeID)
	if respondServiceError(c, err, "Failed to move task") {
		return
	}

	c.JSON(http.StatusOK, task)
}

func (h *TaskHandler) DeleteTask(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	taskID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	err := h.taskService.DeleteTask(c.Request.Context(), userID, taskID)
	if respondServiceError(c, err, "Failed to delete task") {
		return
	}

	c.Status(http.StatusNoContent)
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

	user, err := h.authService.GetUserByID(c.Request.Context(), userID)
	if respondServiceError(c, err, "Failed to fetch user profile") {
		return
	}
	dashboardURL := fmt.Sprintf("%s/dashboard", h.frontendURL)

	err = h.emailService.ShareTasks(c.Request.Context(), req.RecipientEmail, user.Email, dashboardURL)
	if respondServiceError(c, err, "Failed to send email") {
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Task list shared successfully"})
}

// parseTaskFilter reads the board listing query string. Unknown enum values are
// dropped rather than rejected so a stale UI filter never breaks the board.
func parseTaskFilter(c *gin.Context) (models.TaskFilter, bool) {
	filter := models.TaskFilter{Search: strings.TrimSpace(c.Query("q"))}

	for _, raw := range strings.Split(c.Query("status"), ",") {
		if status := models.TaskStatus(strings.ToUpper(strings.TrimSpace(raw))); status.IsValid() {
			filter.Statuses = append(filter.Statuses, status)
		}
	}

	for _, raw := range strings.Split(c.Query("priority"), ",") {
		if priority := models.TaskPriority(strings.ToUpper(strings.TrimSpace(raw))); priority.IsValid() {
			filter.Priorities = append(filter.Priorities, priority)
		}
	}

	if raw := strings.TrimSpace(c.Query("sprint_id")); raw != "" {
		sprintID, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid sprint_id format"})
			return models.TaskFilter{}, false
		}
		filter.SprintID = &sprintID
	}

	if raw := strings.TrimSpace(c.Query("from")); raw != "" {
		from, err := parseTaskTime(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return models.TaskFilter{}, false
		}
		filter.From = &from
	}

	if raw := strings.TrimSpace(c.Query("to")); raw != "" {
		to, err := parseTaskTime(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return models.TaskFilter{}, false
		}
		filter.To = &to
	}

	return filter, true
}
