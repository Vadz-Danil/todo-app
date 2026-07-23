package models

import (
	"time"

	"github.com/google/uuid"
)

// ShareKind selects what a public link exposes.
type ShareKind string

const (
	ShareBoard     ShareKind = "BOARD"
	ShareDashboard ShareKind = "DASHBOARD"
	ShareBoth      ShareKind = "BOTH"
)

func (k ShareKind) IsValid() bool {
	switch k {
	case ShareBoard, ShareDashboard, ShareBoth:
		return true
	}
	return false
}

// IncludesBoard reports whether the kind carries the task list.
func (k ShareKind) IncludesBoard() bool { return k == ShareBoard || k == ShareBoth }

// IncludesDashboard reports whether the kind carries analytics.
func (k ShareKind) IncludesDashboard() bool { return k == ShareDashboard || k == ShareBoth }

// ShareLink is a revocable, optionally expiring capability to read one user's
// board or dashboard without signing in.
//
// Token is set only on creation, when the caller is shown the link once. It is
// never persisted or returned afterwards — the database holds TokenHash only.
type ShareLink struct {
	ID           uuid.UUID  `json:"id"`
	UserID       uuid.UUID  `json:"user_id"`
	Token        string     `json:"token,omitempty"`
	TokenHash    string     `json:"-"`
	Kind         ShareKind  `json:"kind"`
	Label        string     `json:"label"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	ViewCount    int        `json:"view_count"`
	LastViewedAt *time.Time `json:"last_viewed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Active reports whether the link would be accepted right now.
func (s ShareLink) Active(now time.Time) bool {
	if s.RevokedAt != nil {
		return false
	}
	if s.ExpiresAt != nil && !s.ExpiresAt.After(now) {
		return false
	}
	return true
}

// SharedTask is the public projection of a task.
//
// It is a separate type rather than a reused Task on purpose: the public
// payload has to be an explicit allowlist, so that adding a private field to
// Task later cannot silently publish it to everyone holding a link.
type SharedTask struct {
	ID            uuid.UUID    `json:"id"`
	Title         string       `json:"title"`
	Description   *string      `json:"description,omitempty"`
	Status        TaskStatus   `json:"status"`
	Priority      TaskPriority `json:"priority"`
	Reviewer      *string      `json:"reviewer,omitempty"`
	EstimateHours *float64     `json:"estimate_hours,omitempty"`
	BufferHours   *float64     `json:"buffer_hours,omitempty"`
	SpentHours    *float64     `json:"spent_hours,omitempty"`
	Blockers      *string      `json:"blockers,omitempty"`
	DueDate       *time.Time   `json:"due_date,omitempty"`
	CompletedAt   *time.Time   `json:"completed_at,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
}

// NewSharedTask projects a task onto its public form.
func NewSharedTask(t Task) SharedTask {
	return SharedTask{
		ID:            t.ID,
		Title:         t.Title,
		Description:   t.Description,
		Status:        t.Status,
		Priority:      t.Priority,
		Reviewer:      t.Reviewer,
		EstimateHours: t.EstimateHours,
		BufferHours:   t.BufferHours,
		SpentHours:    t.SpentHours,
		Blockers:      t.Blockers,
		DueDate:       t.DueDate,
		CompletedAt:   t.CompletedAt,
		CreatedAt:     t.CreatedAt,
	}
}

// SharedBoardColumn is one kanban column in a public view.
type SharedBoardColumn struct {
	Status TaskStatus   `json:"status"`
	Tasks  []SharedTask `json:"tasks"`
}

// SharedView is the payload served for a valid public link. It deliberately
// carries no identifiers a reader could act on — no user id, no export
// targets, no tokens — only what the owner chose to publish.
type SharedView struct {
	Kind        ShareKind           `json:"kind"`
	Label       string              `json:"label"`
	OwnerEmail  string              `json:"owner_email"`
	GeneratedAt time.Time           `json:"generated_at"`
	ExpiresAt   *time.Time          `json:"expires_at,omitempty"`
	Columns     []SharedBoardColumn `json:"columns,omitempty"`
	TaskCount   int                 `json:"task_count"`
	// Analytics carries its own Range, so the view does not repeat it.
	Analytics *Dashboard `json:"analytics,omitempty"`
}
