package models

import (
	"time"

	"github.com/google/uuid"
)

type SprintStatus string

const (
	SprintPlanned   SprintStatus = "PLANNED"
	SprintActive    SprintStatus = "ACTIVE"
	SprintCompleted SprintStatus = "COMPLETED"
	SprintArchived  SprintStatus = "ARCHIVED"
)

func (s SprintStatus) IsValid() bool {
	switch s {
	case SprintPlanned, SprintActive, SprintCompleted, SprintArchived:
		return true
	}
	return false
}

type Sprint struct {
	ID            uuid.UUID    `json:"id"`
	UserID        uuid.UUID    `json:"user_id"`
	Name          string       `json:"name"`
	Goal          *string      `json:"goal,omitempty"`
	StartsOn      time.Time    `json:"starts_on"`
	EndsOn        time.Time    `json:"ends_on"`
	CapacityHours *float64     `json:"capacity_hours,omitempty"`
	Status        SprintStatus `json:"status"`
	AIRationale   *string      `json:"ai_rationale,omitempty"`
	AIModel       *string      `json:"ai_model,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`

	// Populated on detail reads.
	Tasks []Task        `json:"tasks,omitempty"`
	Stats *SprintStats  `json:"stats,omitempty"`
}

type SprintStats struct {
	TotalTasks     int     `json:"total_tasks"`
	DoneTasks      int     `json:"done_tasks"`
	PlannedHours   float64 `json:"planned_hours"`
	BufferHours    float64 `json:"buffer_hours"`
	CommittedHours float64 `json:"committed_hours"`
	CompletionRate float64 `json:"completion_rate"`
	LoadPercent    float64 `json:"load_percent"`
}
