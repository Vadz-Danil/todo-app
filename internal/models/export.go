package models

import (
	"time"

	"github.com/google/uuid"
)

// ExportSchema is the version tag every outbound envelope carries so the
// receiving server can evolve independently of this app.
const ExportSchema = "todo-app.export.v1"

type ExportKind string

const (
	ExportAnalytics  ExportKind = "ANALYTICS_SNAPSHOT"
	ExportTasks      ExportKind = "TASKS"
	ExportSprints    ExportKind = "SPRINTS"
	ExportAISummary  ExportKind = "AI_SUMMARY"
	ExportFull       ExportKind = "FULL"
)

func (k ExportKind) IsValid() bool {
	switch k {
	case ExportAnalytics, ExportTasks, ExportSprints, ExportAISummary, ExportFull:
		return true
	}
	return false
}

// ExportTarget is a remote HTTP endpoint the user wants snapshots pushed to.
type ExportTarget struct {
	ID         uuid.UUID         `json:"id"`
	UserID     uuid.UUID         `json:"user_id"`
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Secret     string            `json:"-"`
	HasSecret  bool              `json:"has_secret"`
	Headers    map[string]string `json:"headers"`
	Enabled    bool              `json:"enabled"`
	LastStatus *int              `json:"last_status,omitempty"`
	LastError  *string           `json:"last_error,omitempty"`
	LastSentAt *time.Time        `json:"last_sent_at,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

type ExportDelivery struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	TargetID    *uuid.UUID `json:"target_id,omitempty"`
	URL         string     `json:"url"`
	Kind        ExportKind `json:"kind"`
	Status      string     `json:"status"`
	StatusCode  *int       `json:"status_code,omitempty"`
	Attempts    int        `json:"attempts"`
	DurationMS  int        `json:"duration_ms"`
	PayloadSize int        `json:"payload_size"`
	Error       *string    `json:"error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ExportUser identifies the owner of an envelope without leaking credentials.
type ExportUser struct {
	ID    uuid.UUID `json:"id"`
	Email string    `json:"email"`
}

// ExportEnvelope is the exact JSON body POSTed to a target.
type ExportEnvelope struct {
	Schema      string         `json:"schema"`
	Kind        ExportKind     `json:"kind"`
	GeneratedAt time.Time      `json:"generated_at"`
	Nonce       string         `json:"nonce"`
	User        ExportUser     `json:"user"`
	Range       *RangeInfo     `json:"range,omitempty"`
	Analytics   *Dashboard     `json:"analytics,omitempty"`
	Tasks       []Task         `json:"tasks,omitempty"`
	Sprints     []Sprint       `json:"sprints,omitempty"`
	Summary     *AISummary     `json:"ai_summary,omitempty"`
	Counts      map[string]int `json:"counts,omitempty"`
}
