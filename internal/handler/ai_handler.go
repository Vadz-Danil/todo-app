package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/gin-gonic/gin"
)

type AIHandler struct {
	ai        service.AI
	analytics service.Analytics
	auth      service.Auth
}

func NewAIHandler(ai service.AI, analytics service.Analytics, auth service.Auth) *AIHandler {
	return &AIHandler{
		ai:        ai,
		analytics: analytics,
		auth:      auth,
	}
}

type AISummaryRequest struct {
	Period      string `json:"period"`
	From        string `json:"from"`
	To          string `json:"to"`
	Granularity string `json:"granularity"`
	TZ          string `json:"tz"`
	Lang        string `json:"lang"`
	Refresh     bool   `json:"refresh"`
}

type AIPlanningStartRequest struct {
	RawTasks             []string `json:"raw_tasks"`
	Notes                string   `json:"notes"`
	HorizonWeeks         int      `json:"horizon_weeks"`
	CapacityHoursPerWeek float64  `json:"capacity_hours_per_week"`
	StartsOn             string   `json:"starts_on"`
	IncludeBacklog       bool     `json:"include_backlog"`
	Lang                 string   `json:"lang"`
}

type AIPlanningAnswerItem struct {
	QuestionID string `json:"question_id"`
	Answer     string `json:"answer"`
}

type AIPlanningAnswersRequest struct {
	Answers []AIPlanningAnswerItem `json:"answers"`
}

// Status is the one AI route that stays available when the integration is off,
// so the UI can hide the feature instead of showing an error.
func (h *AIHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"enabled": h.ai.Enabled(),
		"model":   h.ai.Model(),
	})
}

func (h *AIHandler) Summary(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	var req AISummaryRequest
	// A missing body is legal here and means "use the default window".
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	q, err := h.analytics.ResolveQuery(req.Period, req.From, req.To, req.Granularity, req.TZ)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.auth.GetUserByID(c.Request.Context(), userID)
	if respondServiceError(c, err, "Failed to load profile") {
		return
	}

	summary, err := h.ai.Summary(c.Request.Context(), user, q, req.Lang, req.Refresh)
	if respondServiceError(c, err, "Failed to generate summary") {
		return
	}

	c.JSON(http.StatusOK, summary)
}

func (h *AIHandler) ListSummaries(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	summaries, err := h.ai.ListSummaries(c.Request.Context(), userID, queryLimit(c, 20, 100))
	if respondServiceError(c, err, "Failed to load summaries") {
		return
	}
	if summaries == nil {
		summaries = []models.AISummary{}
	}

	c.JSON(http.StatusOK, gin.H{"summaries": summaries})
}

func (h *AIHandler) StartPlanning(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	var req AIPlanningStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	startsOn, ok := aiParseDate(c, "starts_on", req.StartsOn)
	if !ok {
		return
	}

	in := service.PlanningStart{
		RawTasks:             req.RawTasks,
		Notes:                req.Notes,
		HorizonWeeks:         req.HorizonWeeks,
		CapacityHoursPerWeek: req.CapacityHoursPerWeek,
		StartsOn:             startsOn,
		IncludeBacklog:       req.IncludeBacklog,
		Lang:                 req.Lang,
	}
	if in.HorizonWeeks <= 0 {
		in.HorizonWeeks = 1
	}
	if in.CapacityHoursPerWeek <= 0 {
		in.CapacityHoursPerWeek = 40
	}

	session, err := h.ai.StartPlanning(c.Request.Context(), userID, in)
	if respondServiceError(c, err, "Failed to start planning session") {
		return
	}

	c.JSON(http.StatusCreated, session)
}

func (h *AIHandler) ListSessions(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	sessions, err := h.ai.ListSessions(c.Request.Context(), userID, queryLimit(c, 20, 100))
	if respondServiceError(c, err, "Failed to load planning sessions") {
		return
	}
	if sessions == nil {
		sessions = []models.PlanningSession{}
	}

	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func (h *AIHandler) GetSession(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	sessionID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	session, err := h.ai.GetSession(c.Request.Context(), userID, sessionID)
	if respondServiceError(c, err, "Failed to load planning session") {
		return
	}

	c.JSON(http.StatusOK, session)
}

func (h *AIHandler) AnswerPlanning(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	sessionID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	var req AIPlanningAnswersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}
	if len(req.Answers) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one answer is required"})
		return
	}

	answers := make([]service.PlanningAnswer, 0, len(req.Answers))
	for _, a := range req.Answers {
		answers = append(answers, service.PlanningAnswer{QuestionID: a.QuestionID, Answer: a.Answer})
	}

	session, err := h.ai.AnswerPlanning(c.Request.Context(), userID, sessionID, answers)
	if respondServiceError(c, err, "Failed to record planning answers") {
		return
	}

	c.JSON(http.StatusOK, session)
}

func (h *AIHandler) GeneratePlan(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	sessionID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	session, err := h.ai.GeneratePlan(c.Request.Context(), userID, sessionID)
	if respondServiceError(c, err, "Failed to generate plan") {
		return
	}

	c.JSON(http.StatusOK, session)
}

func (h *AIHandler) CommitPlan(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	sessionID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	sprint, tasks, err := h.ai.CommitPlan(c.Request.Context(), userID, sessionID)
	if respondServiceError(c, err, "Failed to commit plan") {
		return
	}
	if tasks == nil {
		tasks = []models.Task{}
	}

	c.JSON(http.StatusOK, gin.H{"sprint": sprint, "tasks": tasks})
}

func (h *AIHandler) DeleteSession(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}
	sessionID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	err := h.ai.DeleteSession(c.Request.Context(), userID, sessionID)
	if respondServiceError(c, err, "Failed to delete planning session") {
		return
	}

	c.Status(http.StatusNoContent)
}

// aiParseDate accepts "2006-01-02" or RFC3339; an empty value means "unset".
func aiParseDate(c *gin.Context, field, raw string) (*time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, true
	}
	if t, err := time.Parse(time.DateOnly, raw); err == nil {
		return &t, true
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return &t, true
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid " + field + " format, expected YYYY-MM-DD or RFC3339"})
	return nil, false
}
