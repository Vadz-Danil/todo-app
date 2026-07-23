package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"

	"github.com/google/uuid"
)

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type GoogleLoginRequest struct {
	Code string `json:"code" binding:"required"`
	// RedirectURI is sent by the redirect flow and must match the value the
	// browser used. The popup flow omits it and the server default applies.
	RedirectURI string `json:"redirect_uri"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type ShareTasksRequest struct {
	RecipientEmail string `json:"recipient_email" binding:"required,email"`
}

// Nullable tells an absent JSON field apart from one explicitly sent as null:
// Set reports presence, and a nil Value on a set field means null.
type Nullable[T any] struct {
	Set   bool
	Value *T
}

func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	n.Set = true

	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		n.Value = nil
		return nil
	}

	var value T
	if err := json.Unmarshal(b, &value); err != nil {
		return err
	}
	n.Value = &value

	return nil
}

type CreateTaskRequest struct {
	Title         string               `json:"title" binding:"required"`
	Description   *string              `json:"description"`
	Status        *models.TaskStatus   `json:"status"`
	Priority      *models.TaskPriority `json:"priority"`
	Reviewer      *string              `json:"reviewer"`
	EstimateHours *float64             `json:"estimate_hours"`
	BufferHours   *float64             `json:"buffer_hours"`
	Blockers      *string              `json:"blockers"`
	SprintID      *string              `json:"sprint_id"`
	DueDate       *string              `json:"due_date"`
}

func (r CreateTaskRequest) toCreate() (models.TaskCreate, error) {
	in := models.TaskCreate{
		Title:         r.Title,
		Description:   r.Description,
		Reviewer:      r.Reviewer,
		EstimateHours: r.EstimateHours,
		BufferHours:   r.BufferHours,
		Blockers:      r.Blockers,
	}

	if r.Status != nil {
		in.Status = *r.Status
	}
	if r.Priority != nil {
		in.Priority = *r.Priority
	}

	sprintID, err := optionalSprintID(r.SprintID)
	if err != nil {
		return models.TaskCreate{}, err
	}
	in.SprintID = sprintID

	dueDate, err := optionalDueDate(r.DueDate)
	if err != nil {
		return models.TaskCreate{}, err
	}
	in.DueDate = dueDate

	return in, nil
}

type UpdateTaskRequest struct {
	Title         Nullable[string]              `json:"title"`
	Description   Nullable[string]              `json:"description"`
	Status        Nullable[models.TaskStatus]   `json:"status"`
	Priority      Nullable[models.TaskPriority] `json:"priority"`
	Reviewer      Nullable[string]              `json:"reviewer"`
	EstimateHours Nullable[float64]             `json:"estimate_hours"`
	BufferHours   Nullable[float64]             `json:"buffer_hours"`
	SpentHours    Nullable[float64]             `json:"spent_hours"`
	Blockers      Nullable[string]              `json:"blockers"`
	SprintID      Nullable[string]              `json:"sprint_id"`
	DueDate       Nullable[string]              `json:"due_date"`
}

func (r UpdateTaskRequest) toPatch() (models.TaskPatch, error) {
	patch := models.TaskPatch{
		Description:   patchField(r.Description),
		Reviewer:      patchField(r.Reviewer),
		EstimateHours: patchField(r.EstimateHours),
		BufferHours:   patchField(r.BufferHours),
		SpentHours:    patchField(r.SpentHours),
		Blockers:      patchField(r.Blockers),
	}

	if r.Title.Set {
		if r.Title.Value == nil {
			return models.TaskPatch{}, apperrors.ErrEmptyTaskTitle
		}
		patch.Title = r.Title.Value
	}

	if r.Status.Set {
		if r.Status.Value == nil {
			return models.TaskPatch{}, apperrors.ErrInvalidTaskStatus
		}
		patch.Status = r.Status.Value
	}

	if r.Priority.Set {
		if r.Priority.Value == nil {
			return models.TaskPatch{}, apperrors.ErrInvalidTaskPriority
		}
		patch.Priority = r.Priority.Value
	}

	if r.SprintID.Set {
		sprintID, err := optionalSprintID(r.SprintID.Value)
		if err != nil {
			return models.TaskPatch{}, err
		}
		patch.SprintID = &sprintID
	}

	if r.DueDate.Set {
		dueDate, err := optionalDueDate(r.DueDate.Value)
		if err != nil {
			return models.TaskPatch{}, err
		}
		patch.DueDate = &dueDate
	}

	return patch, nil
}

type UpdateTaskStatusRequest struct {
	Status   models.TaskStatus `json:"status" binding:"required"`
	Reviewer *string           `json:"reviewer"`
}

type MoveTaskRequest struct {
	Status   models.TaskStatus `json:"status" binding:"required"`
	AfterID  *uuid.UUID        `json:"after_id"`
	BeforeID *uuid.UUID        `json:"before_id"`
}

// patchField translates a request field into the models.TaskPatch convention:
// nil leaves the column alone, a non-nil pointer to a nil value clears it.
func patchField[T any](field Nullable[T]) **T {
	if !field.Set {
		return nil
	}
	return &field.Value
}

// optionalSprintID treats null and blank alike, so both clear the sprint link.
func optionalSprintID(raw *string) (*uuid.UUID, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}

	sprintID, err := uuid.Parse(strings.TrimSpace(*raw))
	if err != nil {
		return nil, errors.New("invalid sprint_id: expected a UUID")
	}

	return &sprintID, nil
}

func optionalDueDate(raw *string) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}

	dueDate, err := parseTaskTime(*raw)
	if err != nil {
		return nil, err
	}

	return &dueDate, nil
}

// parseTaskTime accepts a full RFC3339 timestamp or a bare calendar date.
func parseTaskTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)

	if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
		return parsed, nil
	}

	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date %q: expected RFC3339 or YYYY-MM-DD", raw)
	}

	return parsed, nil
}
