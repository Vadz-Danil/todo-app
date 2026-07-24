package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	"todo-app/internal/models"

	"github.com/google/uuid"
)

// analyticsDayFormat is the calendar-day key shared by the heatmap and
// streak calculations.
const analyticsDayFormat = "2006-01-02"

type AnalyticsPostgres struct {
	db *sql.DB
}

func NewAnalyticsPostgres(db *sql.DB) *AnalyticsPostgres {
	return &AnalyticsPostgres{db: db}
}

func (r *AnalyticsPostgres) LoadWindow(ctx context.Context, userID uuid.UUID, from, to time.Time) (tasks []models.Task, err error) {
	query := `
        SELECT ` + taskColumns + `
        FROM tasks
        WHERE user_id = $1
          AND created_at <= $3
          AND (completed_at IS NULL OR completed_at >= $2)
        ORDER BY created_at ASC
    `
	rows, err := r.db.QueryContext(ctx, query, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query analytics window: %w", err)
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

func (r *AnalyticsPostgres) GlobalCounts(ctx context.Context, userID uuid.UUID) (total int, counts map[models.TaskStatus]int, err error) {
	query := `SELECT status, COUNT(*) FROM tasks WHERE user_id = $1 GROUP BY status`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to query global task counts: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	counts = make(map[models.TaskStatus]int, len(models.BoardStatuses))
	for rows.Next() {
		var status models.TaskStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return 0, nil, fmt.Errorf("failed to scan status count: %w", err)
		}
		counts[status] = count
		total += count
	}

	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return total, counts, nil
}

// CompletionDays groups completions by the viewer's calendar day.
//
// The grouping deliberately happens in Go rather than via `AT TIME ZONE` in
// SQL. Go and PostgreSQL carry separate copies of the IANA database, and they
// disagree: a browser on an older platform still reports zones by names that
// IANA has since renamed (Europe/Kiev, Asia/Calcutta), Go's tzdata keeps those
// as aliases, but PostgreSQL builds that omit the "backward" links reject them
// with 22023 and fail the whole dashboard. Passing timestamps instead of a zone
// name removes the disagreement entirely.
func (r *AnalyticsPostgres) CompletionDays(ctx context.Context, userID uuid.UUID, loc *time.Location) (days []models.DayCount, err error) {
	query := `
        SELECT completed_at
        FROM tasks
        WHERE user_id = $1 AND completed_at IS NOT NULL
        ORDER BY completed_at ASC
    `
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query completion days: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	stamps := make([]time.Time, 0)
	for rows.Next() {
		var completedAt time.Time
		if err := rows.Scan(&completedAt); err != nil {
			return nil, fmt.Errorf("failed to scan completion day: %w", err)
		}
		stamps = append(stamps, completedAt)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return groupCompletionsByLocalDay(stamps, loc), nil
}

// groupCompletionsByLocalDay counts timestamps per calendar day in loc,
// ascending. A nil loc means UTC.
func groupCompletionsByLocalDay(stamps []time.Time, loc *time.Location) []models.DayCount {
	if loc == nil {
		loc = time.UTC
	}

	counts := make(map[string]int, len(stamps))
	order := make([]string, 0, len(stamps))
	for _, stamp := range stamps {
		day := stamp.In(loc).Format(analyticsDayFormat)
		if _, seen := counts[day]; !seen {
			order = append(order, day)
		}
		counts[day]++
	}

	// The query returns rows already ordered by completed_at, but a zone with a
	// negative offset can pull a later timestamp into an earlier local day, so
	// sort the keys rather than trusting arrival order.
	slices.Sort(order)

	days := make([]models.DayCount, 0, len(order))
	for _, day := range order {
		days = append(days, models.DayCount{Date: day, Count: counts[day]})
	}

	return days
}

func (r *AnalyticsPostgres) StatusChanges(ctx context.Context, userID uuid.UUID, from, to time.Time) (changes []models.StatusChange, err error) {
	query := `
        SELECT id, task_id, user_id, from_status, to_status, changed_at
        FROM task_status_history
        WHERE user_id = $1 AND changed_at BETWEEN $2 AND $3
        ORDER BY changed_at ASC
    `
	rows, err := r.db.QueryContext(ctx, query, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query status changes: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	changes = make([]models.StatusChange, 0)
	for rows.Next() {
		var change models.StatusChange
		if err := rows.Scan(
			&change.ID,
			&change.TaskID,
			&change.UserID,
			&change.FromStatus,
			&change.ToStatus,
			&change.ChangedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan status change: %w", err)
		}
		changes = append(changes, change)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return changes, nil
}

func (r *AnalyticsPostgres) SprintSummaries(ctx context.Context, userID uuid.UUID, from, to time.Time) (items []models.SprintStatsItem, err error) {
	query := `
        SELECT s.id,
               s.name,
               s.status,
               s.starts_on,
               s.ends_on,
               COUNT(t.id),
               COUNT(*) FILTER (WHERE t.status = 'DONE'),
               COALESCE(SUM(t.estimate_hours), 0)
        FROM sprints s
                 LEFT JOIN tasks t ON t.sprint_id = s.id AND t.user_id = s.user_id
        WHERE s.user_id = $1 AND s.starts_on <= $3 AND s.ends_on >= $2
        GROUP BY s.id
        ORDER BY s.starts_on DESC
    `
	rows, err := r.db.QueryContext(ctx, query, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("failed to query sprint summaries: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	items = make([]models.SprintStatsItem, 0)
	for rows.Next() {
		var sprintID uuid.UUID
		var item models.SprintStatsItem
		if err := rows.Scan(
			&sprintID,
			&item.Name,
			&item.Status,
			&item.StartsOn,
			&item.EndsOn,
			&item.TotalTasks,
			&item.DoneTasks,
			&item.PlannedHours,
		); err != nil {
			return nil, fmt.Errorf("failed to scan sprint summary: %w", err)
		}

		item.SprintID = sprintID.String()
		if item.TotalTasks > 0 {
			item.CompletionRate = math.Round(float64(item.DoneTasks)/float64(item.TotalTasks)*10000) / 10000
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return items, nil
}

func (r *AnalyticsPostgres) FirstTaskAt(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	query := `SELECT MIN(created_at) FROM tasks WHERE user_id = $1`

	var first sql.NullTime
	if err := r.db.QueryRowContext(ctx, query, userID).Scan(&first); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query first task time: %w", err)
	}

	if !first.Valid {
		return nil, nil
	}
	return &first.Time, nil
}
