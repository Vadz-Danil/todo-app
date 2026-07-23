package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const taskColumns = "id, user_id, title, description, status, priority, reviewer, estimate_hours, buffer_hours, spent_hours, blockers, sprint_id, due_date, started_at, completed_at, position, created_at, updated_at"

const taskInsertQuery = `
        INSERT INTO tasks (` + taskColumns + `)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`

const (
	// taskPositionStep is the gap left between neighbouring cards so later drops
	// keep finding room between them without a renumber.
	taskPositionStep = 1024.0
	// taskMinPositionGap is the width below which a float64 midpoint stops being
	// worth trusting and the column is renumbered instead.
	taskMinPositionGap = 0.0001
)

// rowScanner is the read surface shared by *sql.Row and *sql.Rows.
type rowScanner interface{ Scan(dest ...any) error }

// taskQueryer is the single-row query surface shared by *sql.DB and *sql.Tx.
type taskQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// scanTask reads one row selected in taskColumns order.
func scanTask(s rowScanner) (models.Task, error) {
	var (
		task     models.Task
		estimate sql.NullString
		buffer   sql.NullString
		spent    sql.NullString
	)

	err := s.Scan(
		&task.ID,
		&task.UserID,
		&task.Title,
		&task.Description,
		&task.Status,
		&task.Priority,
		&task.Reviewer,
		&estimate,
		&buffer,
		&spent,
		&task.Blockers,
		&task.SprintID,
		&task.DueDate,
		&task.StartedAt,
		&task.CompletedAt,
		&task.Position,
		&task.CreatedAt,
		&task.UpdatedAt,
	)
	if err != nil {
		return models.Task{}, err
	}

	if task.EstimateHours, err = nullFloat(estimate); err != nil {
		return models.Task{}, err
	}
	if task.BufferHours, err = nullFloat(buffer); err != nil {
		return models.Task{}, err
	}
	if task.SpentHours, err = nullFloat(spent); err != nil {
		return models.Task{}, err
	}

	return task, nil
}

// nullFloat converts a NUMERIC column, which lib/pq hands back as raw text
// rather than a float, into an optional float.
func nullFloat(v sql.NullString) (*float64, error) {
	if !v.Valid {
		return nil, nil
	}
	f, err := strconv.ParseFloat(v.String, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse numeric column value %q: %w", v.String, err)
	}
	return &f, nil
}

type TaskPostgres struct {
	db *sql.DB
}

func NewTaskPostgres(db *sql.DB) *TaskPostgres {
	return &TaskPostgres{db: db}
}

func (r *TaskPostgres) CreateTask(ctx context.Context, task *models.Task) error {
	if _, err := r.db.ExecContext(ctx, taskInsertQuery, taskInsertArgs(task)...); err != nil {
		return taskWriteError("failed to create task", err)
	}
	return nil
}

func (r *TaskPostgres) CreateTasksBulk(ctx context.Context, tasks []models.Task) (err error) {
	if len(tasks) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin bulk task insert: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("failed to roll back bulk task insert: %w", rbErr))
		}
	}()

	stmt, err := tx.PrepareContext(ctx, taskInsertQuery)
	if err != nil {
		return fmt.Errorf("failed to prepare bulk task insert: %w", err)
	}
	defer func() {
		if closeErr := stmt.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close bulk task statement: %w", closeErr))
		}
	}()

	for i := range tasks {
		if _, execErr := stmt.ExecContext(ctx, taskInsertArgs(&tasks[i])...); execErr != nil {
			return taskWriteError("failed to insert task in bulk", execErr)
		}
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit bulk task insert: %w", err)
	}
	committed = true

	return nil
}

func (r *TaskPostgres) GetTaskByID(ctx context.Context, userID, taskID uuid.UUID) (*models.Task, error) {
	query := `SELECT ` + taskColumns + ` FROM tasks WHERE id = $1 AND user_id = $2`

	task, err := scanTask(r.db.QueryRowContext(ctx, query, taskID, userID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.ErrTaskNotFound
		}
		return nil, fmt.Errorf("failed to get task by id: %w", err)
	}

	return &task, nil
}

func (r *TaskPostgres) ListTasks(ctx context.Context, userID uuid.UUID, filter models.TaskFilter) (tasks []models.Task, err error) {
	conditions := []string{"user_id = $1"}
	args := []any{userID}

	addCondition := func(format string, value any) {
		args = append(args, value)
		conditions = append(conditions, fmt.Sprintf(format, len(args)))
	}

	if len(filter.Statuses) > 0 {
		statuses := make([]string, 0, len(filter.Statuses))
		for _, status := range filter.Statuses {
			statuses = append(statuses, string(status))
		}
		addCondition("status = ANY($%d)", pq.Array(statuses))
	}

	if len(filter.Priorities) > 0 {
		priorities := make([]string, 0, len(filter.Priorities))
		for _, priority := range filter.Priorities {
			priorities = append(priorities, string(priority))
		}
		addCondition("priority = ANY($%d)", pq.Array(priorities))
	}

	if filter.SprintID != nil {
		addCondition("sprint_id = $%d", *filter.SprintID)
	}

	if term := strings.TrimSpace(filter.Search); term != "" {
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term)
		args = append(args, "%"+escaped+"%")
		conditions = append(conditions, fmt.Sprintf("(title ILIKE $%[1]d OR COALESCE(description, '') ILIKE $%[1]d)", len(args)))
	}

	if filter.From != nil {
		addCondition("created_at >= $%d", *filter.From)
	}
	if filter.To != nil {
		addCondition("created_at <= $%d", *filter.To)
	}

	query := `SELECT ` + taskColumns + ` FROM tasks WHERE ` + strings.Join(conditions, " AND ") +
		` ORDER BY position ASC, created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	tasks = make([]models.Task, 0)
	for rows.Next() {
		task, scanErr := scanTask(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan task: %w", scanErr)
		}
		tasks = append(tasks, task)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return tasks, nil
}

func (r *TaskPostgres) UpdateTask(ctx context.Context, userID, taskID uuid.UUID, patch models.TaskPatch) (*models.Task, error) {
	setClauses := make([]string, 0, 15)
	args := make([]any, 0, 16)

	set := func(column string, value any) {
		args = append(args, value)
		setClauses = append(setClauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}

	if patch.Title != nil {
		set("title", *patch.Title)
	}
	if patch.Description != nil {
		set("description", *patch.Description)
	}
	if patch.Status != nil {
		set("status", string(*patch.Status))
	}
	if patch.Priority != nil {
		set("priority", string(*patch.Priority))
	}
	if patch.Reviewer != nil {
		set("reviewer", *patch.Reviewer)
	}
	if patch.EstimateHours != nil {
		set("estimate_hours", *patch.EstimateHours)
	}
	if patch.BufferHours != nil {
		set("buffer_hours", *patch.BufferHours)
	}
	if patch.SpentHours != nil {
		set("spent_hours", *patch.SpentHours)
	}
	if patch.Blockers != nil {
		set("blockers", *patch.Blockers)
	}
	if patch.SprintID != nil {
		set("sprint_id", *patch.SprintID)
	}
	if patch.DueDate != nil {
		set("due_date", *patch.DueDate)
	}
	if patch.Position != nil {
		set("position", *patch.Position)
	}
	if patch.StartedAt != nil {
		set("started_at", *patch.StartedAt)
	}
	if patch.CompletedAt != nil {
		set("completed_at", *patch.CompletedAt)
	}

	if len(setClauses) == 0 {
		return nil, apperrors.ErrNothingToUpdate
	}
	setClauses = append(setClauses, "updated_at = CURRENT_TIMESTAMP")

	args = append(args, taskID, userID)
	query := fmt.Sprintf(
		`UPDATE tasks SET %s WHERE id = $%d AND user_id = $%d RETURNING `+taskColumns,
		strings.Join(setClauses, ", "), len(args)-1, len(args),
	)

	task, err := scanTask(r.db.QueryRowContext(ctx, query, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.ErrTaskNotFound
		}
		return nil, taskWriteError("failed to update task", err)
	}

	return &task, nil
}

func (r *TaskPostgres) DeleteTask(ctx context.Context, userID, taskID uuid.UUID) error {
	query := `DELETE FROM tasks WHERE id = $1 AND user_id = $2 RETURNING id`

	var deletedID uuid.UUID
	if err := r.db.QueryRowContext(ctx, query, taskID, userID).Scan(&deletedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperrors.ErrTaskNotFound
		}
		return fmt.Errorf("failed to delete task: %w", err)
	}

	return nil
}

func (r *TaskPostgres) AppendPosition(ctx context.Context, userID uuid.UUID, status models.TaskStatus) (float64, error) {
	query := `SELECT COALESCE(MAX(position), 0) + $1 FROM tasks WHERE user_id = $2 AND status = $3`

	var position float64
	if err := r.db.QueryRowContext(ctx, query, taskPositionStep, userID, string(status)).Scan(&position); err != nil {
		return 0, fmt.Errorf("failed to compute append position: %w", err)
	}

	return position, nil
}

func (r *TaskPostgres) PositionBetween(ctx context.Context, userID uuid.UUID, status models.TaskStatus, afterID, beforeID *uuid.UUID) (float64, error) {
	afterPos, err := r.neighbourPosition(ctx, r.db, userID, status, afterID)
	if err != nil {
		return 0, err
	}
	beforePos, err := r.neighbourPosition(ctx, r.db, userID, status, beforeID)
	if err != nil {
		return 0, err
	}

	switch {
	case afterPos == nil && beforePos == nil:
		return r.AppendPosition(ctx, userID, status)
	case afterPos == nil:
		return *beforePos - taskPositionStep, nil
	case beforePos == nil:
		return *afterPos + taskPositionStep, nil
	case *beforePos-*afterPos >= taskMinPositionGap:
		return (*afterPos + *beforePos) / 2, nil
	default:
		return r.rebalanceColumn(ctx, userID, status, *afterID, *beforeID)
	}
}

func (r *TaskPostgres) RecordStatusChange(ctx context.Context, change *models.StatusChange) error {
	if change.ID == uuid.Nil {
		change.ID = uuid.New()
	}
	if change.ChangedAt.IsZero() {
		change.ChangedAt = time.Now()
	}

	query := `
        INSERT INTO task_status_history (id, task_id, user_id, from_status, to_status, changed_at)
        VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.db.ExecContext(ctx, query,
		change.ID,
		change.TaskID,
		change.UserID,
		change.FromStatus,
		string(change.ToStatus),
		change.ChangedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to record status change: %w", err)
	}

	return nil
}

// neighbourPosition returns the position of a neighbouring card, or nil when the
// id is absent or no longer sits in the given column.
func (r *TaskPostgres) neighbourPosition(ctx context.Context, q taskQueryer, userID uuid.UUID, status models.TaskStatus, id *uuid.UUID) (*float64, error) {
	if id == nil {
		return nil, nil
	}

	query := `SELECT position FROM tasks WHERE id = $1 AND user_id = $2 AND status = $3`

	var position float64
	if err := q.QueryRowContext(ctx, query, *id, userID, string(status)).Scan(&position); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load neighbour position: %w", err)
	}

	return &position, nil
}

// rebalanceColumn spreads a column whose neighbours have run out of room back
// onto an evenly spaced grid, then returns the midpoint of the refreshed pair.
func (r *TaskPostgres) rebalanceColumn(ctx context.Context, userID uuid.UUID, status models.TaskStatus, afterID, beforeID uuid.UUID) (position float64, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin position rebalance: %w", err)
	}

	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			err = errors.Join(err, fmt.Errorf("failed to roll back position rebalance: %w", rbErr))
		}
	}()

	renumber := `
        UPDATE tasks AS t
        SET position = ranked.row_number * $1::DOUBLE PRECISION
        FROM (
            SELECT id, ROW_NUMBER() OVER (ORDER BY position ASC, created_at DESC) AS row_number
            FROM tasks
            WHERE user_id = $2 AND status = $3
        ) AS ranked
        WHERE t.id = ranked.id`

	if _, err = tx.ExecContext(ctx, renumber, taskPositionStep, userID, string(status)); err != nil {
		return 0, fmt.Errorf("failed to renumber column positions: %w", err)
	}

	afterPos, err := r.neighbourPosition(ctx, tx, userID, status, &afterID)
	if err != nil {
		return 0, err
	}
	beforePos, err := r.neighbourPosition(ctx, tx, userID, status, &beforeID)
	if err != nil {
		return 0, err
	}
	if afterPos == nil || beforePos == nil {
		return 0, apperrors.ErrTaskNotFound
	}

	position = (*afterPos + *beforePos) / 2

	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit position rebalance: %w", err)
	}
	committed = true

	return position, nil
}

func taskInsertArgs(task *models.Task) []any {
	return []any{
		task.ID,
		task.UserID,
		task.Title,
		task.Description,
		string(task.Status),
		string(task.Priority),
		task.Reviewer,
		task.EstimateHours,
		task.BufferHours,
		task.SpentHours,
		task.Blockers,
		task.SprintID,
		task.DueDate,
		task.StartedAt,
		task.CompletedAt,
		task.Position,
		task.CreatedAt,
		task.UpdatedAt,
	}
}

// taskWriteError maps the constraints guarding the tasks table onto domain
// errors and wraps anything else with the failing operation.
func taskWriteError(op string, err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "23514": // check_violation
			switch pqErr.Constraint {
			case "tasks_reviewer_required":
				return apperrors.ErrReviewerRequired
			case "tasks_status_check":
				return apperrors.ErrInvalidTaskStatus
			case "tasks_priority_check":
				return apperrors.ErrInvalidTaskPriority
			}
		case "23503": // foreign_key_violation
			// The only FK a caller can steer is sprint_id; a sprint that is
			// gone (or was never theirs) is a bad request, not a server fault.
			if strings.Contains(pqErr.Constraint, "sprint") {
				return apperrors.ErrSprintNotOwned
			}
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}
