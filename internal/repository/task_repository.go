package repository

import (
	"context"
	"database/sql"
	"errors"
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

func (r *TaskPostgres) GetTasksByUserID(ctx context.Context, userID uuid.UUID) ([]models.Task, error) {
	query := `SELECT id, user_id, title,description, status, created_at FROM tasks WHERE user_id = $1 ORDER BY created_at DESC`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks := make([]models.Task, 0)
	for rows.Next() {
		var t models.Task
		if err := rows.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Status, &t.CreatedAt); err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (r *TaskPostgres) UpdateTaskStatus(ctx context.Context, id string, userID uuid.UUID, status models.TaskStatus) error {
	query := `UPDATE tasks SET status = $1 WHERE id = $2 AND user_id = $3`
	res, err := r.db.ExecContext(ctx, query, status, id, userID)
	if err != nil {
		return err
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("task not found or permission denied")
	}
	return nil
}
