package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"todo-app/internal/apperrors"
	"todo-app/internal/models"

	"github.com/google/uuid"
)

type TaskRepository interface {
	CreateTask(ctx context.Context, task *models.Task) error
	GetTasksByUserID(ctx context.Context, userID uuid.UUID) ([]models.Task, error)
	UpdateTaskStatus(ctx context.Context, id string, userID uuid.UUID, status models.TaskStatus) error
}

type TaskPostgres struct {
	db *sql.DB
}

func NewTaskPostgres(db *sql.DB) *TaskPostgres {
	return &TaskPostgres{db: db}
}

func (r *TaskPostgres) CreateTask(ctx context.Context, task *models.Task) error {
	query := `
        INSERT INTO tasks (id, user_id, title, description, status, created_at) 
        VALUES ($1, $2, $3, $4, $5, $6)
    `
	_, err := r.db.ExecContext(ctx, query, task.ID, task.UserID, task.Title, task.Description, task.Status, task.CreatedAt)
	return err
}

func (r *TaskPostgres) GetTasksByUserID(ctx context.Context, userID uuid.UUID) (tasks []models.Task, err error) {
	query := `SELECT id, user_id, title, description, status, created_at FROM tasks WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	tasks = make([]models.Task, 0)
	for rows.Next() {
		var t models.Task
		if err := rows.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return tasks, nil
}

func (r *TaskPostgres) UpdateTaskStatus(ctx context.Context, id string, userID uuid.UUID, status models.TaskStatus) error {
	query := `UPDATE tasks SET status = $1 WHERE id = $2 AND user_id = $3`
	res, err := r.db.ExecContext(ctx, query, status, id, userID)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return apperrors.ErrTaskNotFound
	}
	return nil
}
