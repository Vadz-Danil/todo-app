package handler

import (
	"net/http"
	"strings"
	"time"

	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/gin-gonic/gin"
)

type SprintHandler struct {
	sprints service.Sprint
}

func NewSprintHandler(sprints service.Sprint) *SprintHandler {
	return &SprintHandler{sprints: sprints}
}

type SprintCreateRequest struct {
	Name          string   `json:"name"`
	Goal          *string  `json:"goal"`
	StartsOn      string   `json:"starts_on"`
	EndsOn        string   `json:"ends_on"`
	CapacityHours *float64 `json:"capacity_hours"`
	Status        string   `json:"status"`
}

type SprintUpdateRequest struct {
	Name          *string  `json:"name"`
	Goal          *string  `json:"goal"`
	StartsOn      *string  `json:"starts_on"`
	EndsOn        *string  `json:"ends_on"`
	CapacityHours *float64 `json:"capacity_hours"`
	Status        *string  `json:"status"`
}

func (h *SprintHandler) List(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	sprints, err := h.sprints.List(c.Request.Context(), userID)
	if respondServiceError(c, err, "Failed to load sprints") {
		return
	}
	if sprints == nil {
		sprints = []models.Sprint{}
	}

	c.JSON(http.StatusOK, gin.H{"sprints": sprints})
}

func (h *SprintHandler) Create(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	var req SprintCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	startsOn, ok := sprintParseDate(c, "starts_on", req.StartsOn)
	if !ok {
		return
	}
	endsOn, ok := sprintParseDate(c, "ends_on", req.EndsOn)
	if !ok {
		return
	}

	status := models.SprintStatus(strings.TrimSpace(req.Status))
	if status == "" {
		status = models.SprintPlanned
	}

	sprint, err := h.sprints.Create(c.Request.Context(), userID, service.SprintInput{
		Name:          req.Name,
		Goal:          req.Goal,
		StartsOn:      startsOn,
		EndsOn:        endsOn,
		CapacityHours: req.CapacityHours,
		Status:        status,
	})
	if respondServiceError(c, err, "Failed to create sprint") {
		return
	}

	c.JSON(http.StatusCreated, sprint)
}

func (h *SprintHandler) Get(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	sprintID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	sprint, err := h.sprints.Get(c.Request.Context(), userID, sprintID)
	if respondServiceError(c, err, "Failed to load sprint") {
		return
	}

	c.JSON(http.StatusOK, sprint)
}

func (h *SprintHandler) Update(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	sprintID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	var req SprintUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	current, err := h.sprints.Get(c.Request.Context(), userID, sprintID)
	if respondServiceError(c, err, "Failed to load sprint") {
		return
	}

	// The service validates a whole sprint, so the patch is merged onto the
	// stored one before it is handed over.
	in := service.SprintInput{
		Name:          current.Name,
		Goal:          current.Goal,
		StartsOn:      current.StartsOn,
		EndsOn:        current.EndsOn,
		CapacityHours: current.CapacityHours,
		Status:        current.Status,
	}
	if req.Name != nil {
		in.Name = *req.Name
	}
	if req.Goal != nil {
		in.Goal = req.Goal
	}
	if req.CapacityHours != nil {
		in.CapacityHours = req.CapacityHours
	}
	if req.Status != nil {
		in.Status = models.SprintStatus(strings.TrimSpace(*req.Status))
	}
	if req.StartsOn != nil {
		startsOn, ok := sprintParseDate(c, "starts_on", *req.StartsOn)
		if !ok {
			return
		}
		in.StartsOn = startsOn
	}
	if req.EndsOn != nil {
		endsOn, ok := sprintParseDate(c, "ends_on", *req.EndsOn)
		if !ok {
			return
		}
		in.EndsOn = endsOn
	}

	sprint, err := h.sprints.Update(c.Request.Context(), userID, sprintID, in)
	if respondServiceError(c, err, "Failed to update sprint") {
		return
	}

	c.JSON(http.StatusOK, sprint)
}

func (h *SprintHandler) Delete(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	sprintID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	err := h.sprints.Delete(c.Request.Context(), userID, sprintID)
	if respondServiceError(c, err, "Failed to delete sprint") {
		return
	}

	c.Status(http.StatusNoContent)
}

// sprintParseDate accepts "2006-01-02" or RFC3339.
func sprintParseDate(c *gin.Context, field, raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse(time.DateOnly, raw); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid " + field + " format, expected YYYY-MM-DD or RFC3339"})
	return time.Time{}, false
}
