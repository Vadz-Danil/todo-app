package models

import (
	"time"

	"github.com/google/uuid"
)

type PlanningState string

const (
	PlanningCollecting PlanningState = "COLLECTING"
	PlanningReady      PlanningState = "READY"
	PlanningPlanned    PlanningState = "PLANNED"
	PlanningCommitted  PlanningState = "COMMITTED"
)

// QuestionTopic is what the assistant still needs to know about an item.
type QuestionTopic string

const (
	TopicBlockers   QuestionTopic = "BLOCKERS"
	TopicEstimate   QuestionTopic = "ESTIMATE"
	TopicBuffer     QuestionTopic = "BUFFER"
	TopicScope      QuestionTopic = "SCOPE"
	TopicPriority   QuestionTopic = "PRIORITY"
	TopicDependency QuestionTopic = "DEPENDENCY"
	TopicReview     QuestionTopic = "REVIEW"
)

type PlanningSession struct {
	ID                   uuid.UUID       `json:"id"`
	UserID               uuid.UUID       `json:"user_id"`
	State                PlanningState   `json:"state"`
	HorizonWeeks         int             `json:"horizon_weeks"`
	CapacityHoursPerWeek float64         `json:"capacity_hours_per_week"`
	StartsOn             *time.Time      `json:"starts_on,omitempty"`
	SprintID             *uuid.UUID      `json:"sprint_id,omitempty"`
	AIModel              *string         `json:"ai_model,omitempty"`
	Payload              PlanningPayload `json:"payload"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

type PlanningPayload struct {
	Items        []PlanningItem     `json:"items"`
	Questions    []PlanningQuestion `json:"questions"`
	Messages     []PlanningMessage  `json:"messages"`
	Plan         *SprintPlan        `json:"plan,omitempty"`
	Notes        string             `json:"notes,omitempty"`
	CommittedIDs []string           `json:"committed_ids,omitempty"`
}

// PlanningItem is one raw task the user wants planned, enriched as the Q&A progresses.
type PlanningItem struct {
	Ref            string       `json:"ref"`
	Title          string       `json:"title"`
	Notes          string       `json:"notes,omitempty"`
	Priority       TaskPriority `json:"priority,omitempty"`
	HasBlockers    *bool        `json:"has_blockers,omitempty"`
	Blockers       string       `json:"blockers,omitempty"`
	EstimateHours  *float64     `json:"estimate_hours,omitempty"`
	BufferHours    *float64     `json:"buffer_hours,omitempty"`
	NeedsReview    *bool        `json:"needs_review,omitempty"`
	Reviewer       string       `json:"reviewer,omitempty"`
	DependsOn      []string     `json:"depends_on,omitempty"`
	Resolved       bool         `json:"resolved"`
	ExistingTaskID *uuid.UUID   `json:"existing_task_id,omitempty"`
}

// PlanningQuestion is one clarifying question the assistant asks about one item.
type PlanningQuestion struct {
	ID          string        `json:"id"`
	ItemRef     string        `json:"item_ref"`
	Topic       QuestionTopic `json:"topic"`
	Question    string        `json:"question"`
	Why         string        `json:"why,omitempty"`
	Suggestions []string      `json:"suggestions,omitempty"`
	Answer      string        `json:"answer,omitempty"`
	AnsweredAt  *time.Time    `json:"answered_at,omitempty"`
}

type PlanningMessage struct {
	Role    string    `json:"role"`
	Content string    `json:"content"`
	At      time.Time `json:"at"`
}

// SprintPlan is the assistant's final proposal for the upcoming sprint.
type SprintPlan struct {
	Name               string          `json:"name"`
	Goal               string          `json:"goal"`
	StartsOn           string          `json:"starts_on"`
	EndsOn             string          `json:"ends_on"`
	CapacityHours      float64         `json:"capacity_hours"`
	TotalEstimateHours float64         `json:"total_estimate_hours"`
	TotalBufferHours   float64         `json:"total_buffer_hours"`
	CommittedHours     float64         `json:"committed_hours"`
	LoadPercent        float64         `json:"load_percent"`
	Weeks              []PlannedWeek   `json:"weeks"`
	Deferred           []DeferredItem  `json:"deferred,omitempty"`
	Risks              []string        `json:"risks,omitempty"`
	Rationale          string          `json:"rationale,omitempty"`
	Recommendations    []string        `json:"recommendations,omitempty"`
}

type PlannedWeek struct {
	Index         int           `json:"index"`
	StartsOn      string        `json:"starts_on"`
	EndsOn        string        `json:"ends_on"`
	CapacityHours float64       `json:"capacity_hours"`
	LoadHours     float64       `json:"load_hours"`
	Focus         string        `json:"focus,omitempty"`
	Tasks         []PlannedTask `json:"tasks"`
}

type PlannedTask struct {
	Ref           string           `json:"ref"`
	Title         string           `json:"title"`
	Description   string           `json:"description,omitempty"`
	Priority      TaskPriority     `json:"priority"`
	EstimateHours float64          `json:"estimate_hours"`
	BufferHours   float64          `json:"buffer_hours"`
	Blockers      string           `json:"blockers,omitempty"`
	NeedsReview   bool             `json:"needs_review"`
	Reviewer      string           `json:"reviewer,omitempty"`
	DependsOn     []string         `json:"depends_on,omitempty"`
	Order         int              `json:"order"`
	Subtasks      []PlannedSubtask `json:"subtasks,omitempty"`
}

// PlannedSubtask is the breakdown of a task into sittable chunks.
type PlannedSubtask struct {
	Title         string  `json:"title"`
	EstimateHours float64 `json:"estimate_hours"`
}

type DeferredItem struct {
	Ref    string `json:"ref"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

// AISummary is a generated narrative over a analytics window, cached by fingerprint.
type AISummary struct {
	ID          uuid.UUID        `json:"id"`
	UserID      uuid.UUID        `json:"user_id"`
	Period      string           `json:"period"`
	RangeStart  time.Time        `json:"range_start"`
	RangeEnd    time.Time        `json:"range_end"`
	Fingerprint string           `json:"-"`
	AIModel     *string          `json:"ai_model,omitempty"`
	Content     AISummaryContent `json:"content"`
	CreatedAt   time.Time        `json:"created_at"`
	Cached      bool             `json:"cached"`
}

type AISummaryContent struct {
	Headline        string        `json:"headline"`
	Summary         string        `json:"summary"`
	Highlights      []string      `json:"highlights,omitempty"`
	Risks           []string      `json:"risks,omitempty"`
	Recommendations []string      `json:"recommendations,omitempty"`
	FocusNext       []string      `json:"focus_next,omitempty"`
	Metrics         []AIMetricNote `json:"metrics,omitempty"`
	Trend           string        `json:"trend"`
	Score           int           `json:"score"`
}

type AIMetricNote struct {
	Label   string `json:"label"`
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
}
