package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ExportHandler struct {
	export    service.Export
	analytics service.Analytics
	auth      service.Auth
}

func NewExportHandler(export service.Export, analytics service.Analytics, auth service.Auth) *ExportHandler {
	return &ExportHandler{
		export:    export,
		analytics: analytics,
		auth:      auth,
	}
}

// ExportNullableString separates "field absent" from "field present but empty",
// so a patch can clear a secret without every other patch wiping it.
type ExportNullableString struct {
	Value string
	Set   bool
}

func (s *ExportNullableString) UnmarshalJSON(data []byte) error {
	s.Set = true
	if string(data) == "null" {
		s.Value = ""
		return nil
	}
	return json.Unmarshal(data, &s.Value)
}

// Ptr yields nil when the field was absent and a (possibly empty) pointer when
// the caller sent it.
func (s ExportNullableString) Ptr() *string {
	if !s.Set {
		return nil
	}
	value := s.Value
	return &value
}

type ExportTargetCreateRequest struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Secret  *string           `json:"secret"`
	Headers map[string]string `json:"headers"`
	Enabled *bool             `json:"enabled"`
}

type ExportTargetPatchRequest struct {
	Name    *string              `json:"name"`
	URL     *string              `json:"url"`
	Secret  ExportNullableString `json:"secret"`
	Headers map[string]string    `json:"headers"`
	Enabled *bool                `json:"enabled"`
}

type ExportPushRequest struct {
	TargetID    string            `json:"target_id"`
	URL         string            `json:"url"`
	Secret      string            `json:"secret"`
	Headers     map[string]string `json:"headers"`
	Kind        string            `json:"kind"`
	Period      string            `json:"period"`
	From        string            `json:"from"`
	To          string            `json:"to"`
	Granularity string            `json:"granularity"`
	Timezone    string            `json:"tz"`
}

func (h *ExportHandler) ListTargets(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	targets, err := h.export.ListTargets(c.Request.Context(), userID)
	if respondServiceError(c, err, "Failed to fetch export targets") {
		return
	}
	if targets == nil {
		targets = []models.ExportTarget{}
	}

	c.JSON(http.StatusOK, gin.H{"targets": targets})
}

func (h *ExportHandler) CreateTarget(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	var req ExportTargetCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	endpoint := strings.TrimSpace(req.URL)

	target, err := h.export.CreateTarget(c.Request.Context(), userID, service.TargetInput{
		Name:    &name,
		URL:     &endpoint,
		Secret:  req.Secret,
		Headers: req.Headers,
		Enabled: req.Enabled,
	})
	if respondServiceError(c, err, "Failed to create export target") {
		return
	}

	c.JSON(http.StatusCreated, target)
}

func (h *ExportHandler) UpdateTarget(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	targetID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	var req ExportTargetPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	in := service.TargetInput{
		Secret:  req.Secret.Ptr(),
		Headers: req.Headers,
		Enabled: req.Enabled,
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		in.Name = &name
	}
	if req.URL != nil {
		endpoint := strings.TrimSpace(*req.URL)
		in.URL = &endpoint
	}

	target, err := h.export.UpdateTarget(c.Request.Context(), userID, targetID, in)
	if respondServiceError(c, err, "Failed to update export target") {
		return
	}

	c.JSON(http.StatusOK, target)
}

func (h *ExportHandler) DeleteTarget(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	targetID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	err := h.export.DeleteTarget(c.Request.Context(), userID, targetID)
	if respondServiceError(c, err, "Failed to delete export target") {
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *ExportHandler) Preview(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	kind, ok := exportKindOrDefault(c, c.Query("kind"))
	if !ok {
		return
	}

	q, ok := parseAnalyticsQuery(c, h.analytics)
	if !ok {
		return
	}

	user, ok := h.currentUser(c, userID)
	if !ok {
		return
	}

	envelope, err := h.export.BuildEnvelope(c.Request.Context(), user, kind, q, nil)
	if respondServiceError(c, err, "Failed to build export preview") {
		return
	}

	c.JSON(http.StatusOK, envelope)
}

func (h *ExportHandler) Push(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	var req ExportPushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	rawTargetID := strings.TrimSpace(req.TargetID)
	url := strings.TrimSpace(req.URL)
	if (rawTargetID == "") == (url == "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "exactly one of target_id and url is required"})
		return
	}

	var targetID *uuid.UUID
	if rawTargetID != "" {
		parsed, err := uuid.Parse(rawTargetID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid target_id format"})
			return
		}
		targetID = &parsed
	}

	kind, ok := exportKindOrDefault(c, req.Kind)
	if !ok {
		return
	}

	q, err := h.analytics.ResolveQuery(req.Period, req.From, req.To, req.Granularity, req.Timezone)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, ok := h.currentUser(c, userID)
	if !ok {
		return
	}

	delivery, err := h.export.Push(c.Request.Context(), user, service.PushRequest{
		TargetID: targetID,
		URL:      url,
		Secret:   req.Secret,
		Headers:  req.Headers,
		Kind:     kind,
		Query:    q,
	})
	if err != nil {
		if delivery != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "delivery": delivery})
			return
		}
		respondServiceError(c, err, "Failed to push export")
		return
	}

	c.JSON(http.StatusOK, delivery)
}

func (h *ExportHandler) ListDeliveries(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	deliveries, err := h.export.ListDeliveries(c.Request.Context(), userID, queryLimit(c, 50, 200))
	if respondServiceError(c, err, "Failed to fetch export deliveries") {
		return
	}
	if deliveries == nil {
		deliveries = []models.ExportDelivery{}
	}

	c.JSON(http.StatusOK, gin.H{"deliveries": deliveries})
}

// currentUser resolves the envelope owner, writing a 500 when it cannot.
func (h *ExportHandler) currentUser(c *gin.Context, userID uuid.UUID) (*models.User, bool) {
	user, err := h.auth.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch user profile"})
		return nil, false
	}
	return user, true
}

// exportKindOrDefault falls back to ANALYTICS_SNAPSHOT for an empty value and
// rejects anything else that is not a known kind.
func exportKindOrDefault(c *gin.Context, raw string) (models.ExportKind, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return models.ExportAnalytics, true
	}

	kind := models.ExportKind(strings.ToUpper(trimmed))
	if !kind.IsValid() {
		c.JSON(http.StatusBadRequest, gin.H{"error": apperrors.ErrInvalidExportKind.Error()})
		return "", false
	}

	return kind, true
}
