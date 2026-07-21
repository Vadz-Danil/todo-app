package models

import (
	"time"

	"github.com/google/uuid"
)

type TaskStatus string

const (
	StatusTodo TaskStatus = "TODO"
	StatusDone TaskStatus = "DONE"
)

type Task struct {
	ID          uuid.UUID  `json:"id"`
	UserID      uuid.UUID  `json:"user_id"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Status      TaskStatus `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
}
