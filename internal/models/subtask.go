package models

import (
	"time"

	"github.com/google/uuid"
)

// Subtask is one checklist item inside a task.
type Subtask struct {
	ID     uuid.UUID `json:"id"`
	TaskID uuid.UUID `json:"task_id"`
	// UserID is carried for ownership checks and never serialised — the client
	// already knows whose board it is looking at.
	UserID    uuid.UUID `json:"-"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	Position  float64   `json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SubtaskCreate is the validated input for a new checklist item.
type SubtaskCreate struct {
	Title string
}

// SubtaskPatch is a partial update. A nil field means "leave unchanged".
type SubtaskPatch struct {
	Title *string
	Done  *bool
}

// SubtaskProgress is the done/total rollup the board shows on a card without
// fetching the whole checklist.
type SubtaskProgress struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}
