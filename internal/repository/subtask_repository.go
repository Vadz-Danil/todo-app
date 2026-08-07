package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

const subtaskColumns = "id, task_id, title, done, position, created_at, updated_at"

// subtaskPositionStep leaves room between neighbours so a later insert finds a
// midpoint, mirroring how task positions work on the board.
const subtaskPositionStep = 1024.0

type SubtaskPostgres struct {
	db *sql.DB
}

func NewSubtaskPostgres(db *sql.DB) *SubtaskPostgres {
	return &SubtaskPostgres{db: db}
}

// ListByTask returns a task's checklist in order. It filters by user_id as well
// as task_id so a caller cannot read the checklist of a task they do not own.
func (r *SubtaskPostgres) ListByTask(ctx context.Context, userID, taskID uuid.UUID) (items []models.Subtask, err error) {
	query := `SELECT ` + subtaskColumns + `
              FROM subtasks
              WHERE task_id = $1 AND user_id = $2
              ORDER BY position ASC, created_at ASC`

	rows, err := r.db.QueryContext(ctx, query, taskID, userID)
	if err != nil {
		return nil, fmt.Errorf("list subtasks: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	items = make([]models.Subtask, 0)
	for rows.Next() {
		item, scanErr := scanSubtask(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan subtask: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subtasks: %w", err)
	}

	return items, nil
}

// ProgressByTasks returns the done/total rollup for many tasks in one query, so
// the board can badge every card without an N+1.
func (r *SubtaskPostgres) ProgressByTasks(ctx context.Context, userID uuid.UUID, taskIDs []uuid.UUID) (progress map[uuid.UUID]models.SubtaskProgress, err error) {
	progress = make(map[uuid.UUID]models.SubtaskProgress)
	if len(taskIDs) == 0 {
		return progress, nil
	}

	query := `SELECT task_id,
                     COUNT(*) FILTER (WHERE done)                     AS done,
                     COUNT(*)                                         AS total
              FROM subtasks
              WHERE user_id = $1 AND task_id = ANY($2)
              GROUP BY task_id`

	rows, err := r.db.QueryContext(ctx, query, userID, pq.Array(taskIDs))
	if err != nil {
		return nil, fmt.Errorf("subtask progress: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close rows: %w", closeErr))
		}
	}()

	for rows.Next() {
		var taskID uuid.UUID
		var p models.SubtaskProgress
		if err := rows.Scan(&taskID, &p.Done, &p.Total); err != nil {
			return nil, fmt.Errorf("scan subtask progress: %w", err)
		}
		progress[taskID] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subtask progress: %w", err)
	}

	return progress, nil
}

// NextPosition returns the position for a new item appended to a task's list.
func (r *SubtaskPostgres) NextPosition(ctx context.Context, userID, taskID uuid.UUID) (float64, error) {
	var maxPos sql.NullFloat64
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(position) FROM subtasks WHERE task_id = $1 AND user_id = $2`,
		taskID, userID,
	).Scan(&maxPos)
	if err != nil {
		return 0, fmt.Errorf("next subtask position: %w", err)
	}
	if !maxPos.Valid {
		return subtaskPositionStep, nil
	}
	return maxPos.Float64 + subtaskPositionStep, nil
}

// Create inserts a checklist item. A foreign-key violation means the parent
// task does not exist (or is not the caller's), which is a bad request rather
// than a server fault.
func (r *SubtaskPostgres) Create(ctx context.Context, item *models.Subtask) error {
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}

	query := `INSERT INTO subtasks (id, task_id, user_id, title, done, position)
              VALUES ($1, $2, $3, $4, $5, $6)
              RETURNING created_at, updated_at`

	err := r.db.QueryRowContext(ctx, query,
		item.ID, item.TaskID, item.UserID, item.Title, item.Done, item.Position,
	).Scan(&item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return subtaskWriteError("create subtask", err)
	}

	return nil
}

// Update applies a patch and returns the stored row. Missing (or not-owned)
// rows yield ErrSubtaskNotFound.
func (r *SubtaskPostgres) Update(ctx context.Context, userID, subtaskID uuid.UUID, patch models.SubtaskPatch) (*models.Subtask, error) {
	set := make([]string, 0, 2)
	args := make([]any, 0, 4)
	next := 1

	if patch.Title != nil {
		set = append(set, fmt.Sprintf("title = $%d", next))
		args = append(args, *patch.Title)
		next++
	}
	if patch.Done != nil {
		set = append(set, fmt.Sprintf("done = $%d", next))
		args = append(args, *patch.Done)
		next++
	}

	if len(set) == 0 {
		return nil, apperrors.ErrNothingToUpdate
	}

	set = append(set, "updated_at = CURRENT_TIMESTAMP")

	query := fmt.Sprintf(
		`UPDATE subtasks SET %s WHERE id = $%d AND user_id = $%d RETURNING `+subtaskColumns,
		strings.Join(set, ", "), next, next+1,
	)
	args = append(args, subtaskID, userID)

	item, err := scanSubtask(r.db.QueryRowContext(ctx, query, args...))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.ErrSubtaskNotFound
		}
		return nil, subtaskWriteError("update subtask", err)
	}

	return &item, nil
}

func (r *SubtaskPostgres) Delete(ctx context.Context, userID, subtaskID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM subtasks WHERE id = $1 AND user_id = $2`, subtaskID, userID)
	if err != nil {
		return fmt.Errorf("delete subtask: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete subtask: %w", err)
	}
	if affected == 0 {
		return apperrors.ErrSubtaskNotFound
	}

	return nil
}

func scanSubtask(s rowScanner) (models.Subtask, error) {
	var item models.Subtask
	err := s.Scan(
		&item.ID,
		&item.TaskID,
		&item.Title,
		&item.Done,
		&item.Position,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	return item, err
}

// subtaskWriteError maps the constraints guarding the table onto domain errors.
func subtaskWriteError(op string, err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code {
		case "23514": // check_violation — only the not-blank title guard
			return apperrors.ErrEmptySubtaskTitle
		case "23503": // foreign_key_violation — the parent task is gone
			return apperrors.ErrTaskNotFound
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}
