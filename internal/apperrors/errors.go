package apperrors

import "errors"

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user with this email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrTaskNotFound       = errors.New("task not found or permission denied")
	ErrEmptyTaskTitle     = errors.New("task title cannot be empty")
	ErrInvalidTaskStatus  = errors.New("invalid task status")
	ErrEmptyRecipient     = errors.New("recipient email is required")
	ErrUnauthorized       = errors.New("unauthorized access")
	ErrInvalidRedirectURI = errors.New("redirect_uri is not registered for this application")

	ErrInvalidTaskPriority = errors.New("invalid task priority")
	ErrReviewerRequired    = errors.New("reviewer is required when status is IN_REVIEW")
	ErrNothingToUpdate     = errors.New("no fields to update")
	ErrInvalidTaskHours    = errors.New("hours must be between 0 and 1000")
	ErrTaskTitleTooLong    = errors.New("task title is too long")
	ErrSprintNotOwned      = errors.New("sprint not found or permission denied")

	ErrInvalidPeriod      = errors.New("invalid analytics period")
	ErrInvalidGranularity = errors.New("invalid analytics granularity")
	ErrInvalidDateRange   = errors.New("invalid date range")

	ErrSprintNotFound  = errors.New("sprint not found or permission denied")
	ErrInvalidSprint   = errors.New("invalid sprint payload")
	ErrSessionNotFound = errors.New("planning session not found or permission denied")
	ErrSessionState    = errors.New("planning session is not in the required state")
	ErrNoPlanningItems = errors.New("at least one task is required to start planning")

	ErrAIDisabled    = errors.New("AI features are disabled: GEMINI_API_KEY is not configured")
	ErrAIUnavailable = errors.New("AI provider is temporarily unavailable")
	ErrAIBadResponse = errors.New("AI provider returned an unusable response")

	ErrExportTargetNotFound = errors.New("export target not found or permission denied")
	ErrInvalidExportURL     = errors.New("export URL must be an absolute http(s) URL")
	ErrBlockedExportURL     = errors.New("export URL resolves to a private network address, which is not allowed")
	ErrExportFailed         = errors.New("failed to deliver export payload")
	ErrInvalidExportKind    = errors.New("invalid export kind")
)
