package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"todo-app/internal/apperrors"
	"todo-app/internal/middleware"
	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// getUserIDFromContext pulls the authenticated user out of the gin context,
// writing a 401 and returning false when it is missing or malformed.
func getUserIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	userIDVal, exists := c.Get(middleware.UserIDKey)
	if !exists {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": apperrors.ErrUnauthorized.Error()})
		return uuid.Nil, false
	}

	userID, ok := userIDVal.(uuid.UUID)
	if !ok {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": apperrors.ErrUnauthorized.Error()})
		return uuid.Nil, false
	}

	return userID, true
}

// parsePathUUID reads a UUID path parameter, writing a 400 when it is absent
// or malformed.
func parsePathUUID(c *gin.Context, param string) (uuid.UUID, bool) {
	raw := strings.TrimSpace(c.Param(param))
	if raw == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing " + param})
		return uuid.Nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid " + param + " format"})
		return uuid.Nil, false
	}
	return id, true
}

// parseAnalyticsQuery resolves the shared `period/from/to/granularity/tz`
// query-string contract used by the analytics, AI and export endpoints.
func parseAnalyticsQuery(c *gin.Context, analytics service.Analytics) (models.AnalyticsQuery, bool) {
	q, err := analytics.ResolveQuery(
		c.Query("period"),
		c.Query("from"),
		c.Query("to"),
		c.Query("granularity"),
		c.Query("tz"),
	)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return models.AnalyticsQuery{}, false
	}
	return q, true
}

// queryLimit reads a bounded `limit` query parameter.
func queryLimit(c *gin.Context, fallback, max int) int {
	raw := strings.TrimSpace(c.Query("limit"))
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val <= 0 {
		return fallback
	}
	if val > max {
		return max
	}
	return val
}

// respondServiceError maps domain errors onto HTTP status codes. It returns
// false when err is nil so callers can use it as a guard.
func respondServiceError(c *gin.Context, err error, fallbackMessage string) bool {
	if err == nil {
		return false
	}

	switch {
	case errors.Is(err, apperrors.ErrUnauthorized):
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})

	case errors.Is(err, apperrors.ErrTaskNotFound),
		errors.Is(err, apperrors.ErrSprintNotFound),
		errors.Is(err, apperrors.ErrSessionNotFound),
		errors.Is(err, apperrors.ErrExportTargetNotFound),
		errors.Is(err, apperrors.ErrShareLinkNotFound),
		errors.Is(err, apperrors.ErrUserNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})

	case errors.Is(err, apperrors.ErrEmptyTaskTitle),
		errors.Is(err, apperrors.ErrInvalidTaskStatus),
		errors.Is(err, apperrors.ErrInvalidTaskPriority),
		errors.Is(err, apperrors.ErrInvalidTaskHours),
		errors.Is(err, apperrors.ErrTaskTitleTooLong),
		errors.Is(err, apperrors.ErrSprintNotOwned),
		errors.Is(err, apperrors.ErrReviewerRequired),
		errors.Is(err, apperrors.ErrNothingToUpdate),
		errors.Is(err, apperrors.ErrInvalidPeriod),
		errors.Is(err, apperrors.ErrInvalidGranularity),
		errors.Is(err, apperrors.ErrInvalidDateRange),
		errors.Is(err, apperrors.ErrInvalidSprint),
		errors.Is(err, apperrors.ErrNoPlanningItems),
		errors.Is(err, apperrors.ErrSessionState),
		errors.Is(err, apperrors.ErrInvalidExportURL),
		errors.Is(err, apperrors.ErrBlockedExportURL),
		errors.Is(err, apperrors.ErrInvalidExportKind),
		errors.Is(err, apperrors.ErrInvalidShareKind),
		errors.Is(err, apperrors.ErrShareLabelTooLong),
		errors.Is(err, apperrors.ErrInvalidShareTTL),
		errors.Is(err, apperrors.ErrTooManyShareLinks),
		errors.Is(err, apperrors.ErrEmptyRecipient):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})

	case errors.Is(err, apperrors.ErrAIDisabled):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})

	case errors.Is(err, apperrors.ErrAIUnavailable),
		errors.Is(err, apperrors.ErrAIBadResponse),
		errors.Is(err, apperrors.ErrEmailDelivery),
		errors.Is(err, apperrors.ErrExportFailed):
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})

	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": fallbackMessage})
	}

	return true
}
