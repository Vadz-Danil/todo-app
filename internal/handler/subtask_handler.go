package handler

import (
	"net/http"
	"strings"

	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// maxProgressTaskIDs bounds a batch progress lookup so one request cannot ask
// the database about an unbounded list of ids.
const maxProgressTaskIDs = 500

type SubtaskHandler struct {
	subtasks service.Subtask
}

func NewSubtaskHandler(subtasks service.Subtask) *SubtaskHandler {
	return &SubtaskHandler{subtasks: subtasks}
}

type SubtaskCreateRequest struct {
	Title string `json:"title"`
}

type SubtaskPatchRequest struct {
	Title *string `json:"title"`
	Done  *bool   `json:"done"`
}

// List returns a task's checklist.
func (h *SubtaskHandler) List(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	taskID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	items, err := h.subtasks.List(c.Request.Context(), userID, taskID)
	if respondServiceError(c, err, "Failed to fetch subtasks") {
		return
	}
	if items == nil {
		items = []models.Subtask{}
	}

	c.JSON(http.StatusOK, gin.H{"subtasks": items})
}

// Progress returns the done/total rollup for the given task ids, so the board
// can badge every card in a single request. Ids are passed as a comma-separated
// `task_ids` query parameter; unknown or not-owned ids are simply absent from
// the result.
func (h *SubtaskHandler) Progress(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	raw := strings.TrimSpace(c.Query("task_ids"))
	if raw == "" {
		c.JSON(http.StatusOK, gin.H{"progress": map[string]models.SubtaskProgress{}})
		return
	}

	parts := strings.Split(raw, ",")
	if len(parts) > maxProgressTaskIDs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many task_ids"})
		return
	}

	ids := make([]uuid.UUID, 0, len(parts))
	for _, part := range parts {
		id, err := uuid.Parse(strings.TrimSpace(part))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task_ids"})
			return
		}
		ids = append(ids, id)
	}

	progress, err := h.subtasks.Progress(c.Request.Context(), userID, ids)
	if respondServiceError(c, err, "Failed to fetch subtask progress") {
		return
	}

	// Serialise with string keys so the JSON object is well-formed.
	out := make(map[string]models.SubtaskProgress, len(progress))
	for id, p := range progress {
		out[id.String()] = p
	}

	c.JSON(http.StatusOK, gin.H{"progress": out})
}

// Create appends a checklist item to a task.
func (h *SubtaskHandler) Create(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	taskID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	var req SubtaskCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	item, err := h.subtasks.Create(c.Request.Context(), userID, taskID,
		models.SubtaskCreate{Title: strings.TrimSpace(req.Title)})
	if respondServiceError(c, err, "Failed to create subtask") {
		return
	}

	c.JSON(http.StatusCreated, item)
}

// Update edits a checklist item. The item id is on the path; the task is not
// needed because ownership is enforced by the item's own user_id.
func (h *SubtaskHandler) Update(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	subtaskID, ok := parsePathUUID(c, "subtaskID")
	if !ok {
		return
	}

	var req SubtaskPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	item, err := h.subtasks.Update(c.Request.Context(), userID, subtaskID, models.SubtaskPatch{
		Title: req.Title,
		Done:  req.Done,
	})
	if respondServiceError(c, err, "Failed to update subtask") {
		return
	}

	c.JSON(http.StatusOK, item)
}

func (h *SubtaskHandler) Delete(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	subtaskID, ok := parsePathUUID(c, "subtaskID")
	if !ok {
		return
	}

	if respondServiceError(c, h.subtasks.Delete(c.Request.Context(), userID, subtaskID), "Failed to delete subtask") {
		return
	}

	c.Status(http.StatusNoContent)
}
