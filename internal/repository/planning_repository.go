package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"

	"github.com/google/uuid"
)

const (
	sprintColumns  = `id, user_id, name, goal, starts_on, ends_on, capacity_hours, status, ai_rationale, ai_model, created_at, updated_at`
	sessionColumns = `id, user_id, state, horizon_weeks, capacity_hours_per_week, starts_on, payload, sprint_id, ai_model, created_at, updated_at`
	summaryColumns = `id, user_id, period, range_start, range_end, fingerprint, content, ai_model, created_at`
)

// planningScanner is satisfied by both *sql.Row and *sql.Rows.
type planningScanner interface {
	Scan(dest ...any) error
}

// planningNullFloat converts a NUMERIC column, which lib/pq hands back as text,
// into a float pointer.
func planningNullFloat(raw sql.NullString) (*float64, error) {
	if !raw.Valid {
		return nil, nil
	}
	val, err := strconv.ParseFloat(strings.TrimSpace(raw.String), 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse numeric value %q: %w", raw.String, err)
	}
	return &val, nil
}

func planningListLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}

type SprintPostgres struct {
	db *sql.DB
}

func NewSprintPostgres(db *sql.DB) *SprintPostgres {
	return &SprintPostgres{db: db}
}

func (r *SprintPostgres) CreateSprint(ctx context.Context, sprint *models.Sprint) error {
	if sprint.ID == uuid.Nil {
		sprint.ID = uuid.New()
	}

	query := `
        INSERT INTO sprints (id, user_id, name, goal, starts_on, ends_on, capacity_hours, status, ai_rationale, ai_model)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
        RETURNING created_at, updated_at
    `
	err := r.db.QueryRowContext(ctx, query,
		sprint.ID,
		sprint.UserID,
		sprint.Name,
		sprint.Goal,
		sprint.StartsOn,
		sprint.EndsOn,
		sprint.CapacityHours,
		sprint.Status,
		sprint.AIRationale,
		sprint.AIModel,
	).Scan(&sprint.CreatedAt, &sprint.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create sprint: %w", err)
	}

	return nil
}

func (r *SprintPostgres) GetSprintByID(ctx context.Context, userID, sprintID uuid.UUID) (*models.Sprint, error) {
	query := `SELECT ` + sprintColumns + ` FROM sprints WHERE id = $1 AND user_id = $2`

	sprint, err := scanSprintRow(r.db.QueryRowContext(ctx, query, sprintID, userID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.ErrSprintNotFound
		}
		return nil, fmt.Errorf("failed to get sprint: %w", err)
	}

	return sprint, nil
}

func (r *SprintPostgres) ListSprints(ctx context.Context, userID uuid.UUID) (sprints []models.Sprint, err error) {
	query := `SELECT ` + sprintColumns + ` FROM sprints WHERE user_id = $1 ORDER BY starts_on DESC`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list sprints: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	sprints = make([]models.Sprint, 0)
	for rows.Next() {
		sprint, scanErr := scanSprintRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan sprint: %w", scanErr)
		}
		sprints = append(sprints, *sprint)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return sprints, nil
}

func (r *SprintPostgres) UpdateSprint(ctx context.Context, sprint *models.Sprint) error {
	query := `
        UPDATE sprints
        SET name = $1,
            goal = $2,
            starts_on = $3,
            ends_on = $4,
            capacity_hours = $5,
            status = $6,
            ai_rationale = $7,
            ai_model = $8,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $9 AND user_id = $10
        RETURNING updated_at
    `
	err := r.db.QueryRowContext(ctx, query,
		sprint.Name,
		sprint.Goal,
		sprint.StartsOn,
		sprint.EndsOn,
		sprint.CapacityHours,
		sprint.Status,
		sprint.AIRationale,
		sprint.AIModel,
		sprint.ID,
		sprint.UserID,
	).Scan(&sprint.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperrors.ErrSprintNotFound
		}
		return fmt.Errorf("failed to update sprint: %w", err)
	}

	return nil
}

func (r *SprintPostgres) DeleteSprint(ctx context.Context, userID, sprintID uuid.UUID) error {
	query := `DELETE FROM sprints WHERE id = $1 AND user_id = $2`

	res, err := r.db.ExecContext(ctx, query, sprintID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete sprint: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return apperrors.ErrSprintNotFound
	}

	return nil
}

func scanSprintRow(scanner planningScanner) (*models.Sprint, error) {
	var (
		sprint   models.Sprint
		capacity sql.NullString
	)

	err := scanner.Scan(
		&sprint.ID,
		&sprint.UserID,
		&sprint.Name,
		&sprint.Goal,
		&sprint.StartsOn,
		&sprint.EndsOn,
		&capacity,
		&sprint.Status,
		&sprint.AIRationale,
		&sprint.AIModel,
		&sprint.CreatedAt,
		&sprint.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	sprint.CapacityHours, err = planningNullFloat(capacity)
	if err != nil {
		return nil, err
	}

	return &sprint, nil
}

type PlanningPostgres struct {
	db *sql.DB
}

func NewPlanningPostgres(db *sql.DB) *PlanningPostgres {
	return &PlanningPostgres{db: db}
}

func (r *PlanningPostgres) CreateSession(ctx context.Context, session *models.PlanningSession) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}

	payload, err := encodePlanningPayload(session.Payload)
	if err != nil {
		return err
	}

	query := `
        INSERT INTO planning_sessions (id, user_id, state, horizon_weeks, capacity_hours_per_week, starts_on, payload, sprint_id, ai_model)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        RETURNING created_at, updated_at
    `
	err = r.db.QueryRowContext(ctx, query,
		session.ID,
		session.UserID,
		session.State,
		session.HorizonWeeks,
		session.CapacityHoursPerWeek,
		session.StartsOn,
		payload,
		session.SprintID,
		session.AIModel,
	).Scan(&session.CreatedAt, &session.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create planning session: %w", err)
	}

	return nil
}

func (r *PlanningPostgres) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.PlanningSession, error) {
	query := `SELECT ` + sessionColumns + ` FROM planning_sessions WHERE id = $1 AND user_id = $2`

	session, err := scanSessionRow(r.db.QueryRowContext(ctx, query, sessionID, userID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to get planning session: %w", err)
	}

	return session, nil
}

func (r *PlanningPostgres) ListSessions(ctx context.Context, userID uuid.UUID, limit int) (sessions []models.PlanningSession, err error) {
	query := `SELECT ` + sessionColumns + ` FROM planning_sessions WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`

	rows, err := r.db.QueryContext(ctx, query, userID, planningListLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("failed to list planning sessions: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	sessions = make([]models.PlanningSession, 0)
	for rows.Next() {
		session, scanErr := scanSessionRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan planning session: %w", scanErr)
		}
		sessions = append(sessions, *session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return sessions, nil
}

func (r *PlanningPostgres) UpdateSession(ctx context.Context, session *models.PlanningSession) error {
	payload, err := encodePlanningPayload(session.Payload)
	if err != nil {
		return err
	}

	query := `
        UPDATE planning_sessions
        SET state = $1,
            horizon_weeks = $2,
            capacity_hours_per_week = $3,
            starts_on = $4,
            payload = $5,
            sprint_id = $6,
            ai_model = $7,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = $8 AND user_id = $9
        RETURNING updated_at
    `
	err = r.db.QueryRowContext(ctx, query,
		session.State,
		session.HorizonWeeks,
		session.CapacityHoursPerWeek,
		session.StartsOn,
		payload,
		session.SprintID,
		session.AIModel,
		session.ID,
		session.UserID,
	).Scan(&session.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperrors.ErrSessionNotFound
		}
		return fmt.Errorf("failed to update planning session: %w", err)
	}

	return nil
}

func (r *PlanningPostgres) DeleteSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	query := `DELETE FROM planning_sessions WHERE id = $1 AND user_id = $2`

	res, err := r.db.ExecContext(ctx, query, sessionID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete planning session: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return apperrors.ErrSessionNotFound
	}

	return nil
}

func scanSessionRow(scanner planningScanner) (*models.PlanningSession, error) {
	var (
		session  models.PlanningSession
		capacity sql.NullString
		payload  []byte
		sprintID uuid.NullUUID
	)

	err := scanner.Scan(
		&session.ID,
		&session.UserID,
		&session.State,
		&session.HorizonWeeks,
		&capacity,
		&session.StartsOn,
		&payload,
		&sprintID,
		&session.AIModel,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	capacityHours, err := planningNullFloat(capacity)
	if err != nil {
		return nil, err
	}
	if capacityHours != nil {
		session.CapacityHoursPerWeek = *capacityHours
	}

	if sprintID.Valid {
		session.SprintID = &sprintID.UUID
	}

	session.Payload, err = decodePlanningPayload(payload)
	if err != nil {
		return nil, err
	}

	return &session, nil
}

// encodePlanningPayload returns a string because lib/pq would otherwise send a
// []byte as bytea, which Postgres refuses to cast to jsonb.
func encodePlanningPayload(payload models.PlanningPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal planning payload: %w", err)
	}
	return string(raw), nil
}

func decodePlanningPayload(raw []byte) (models.PlanningPayload, error) {
	var payload models.PlanningPayload
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			return models.PlanningPayload{}, fmt.Errorf("failed to unmarshal planning payload: %w", err)
		}
	}

	if payload.Items == nil {
		payload.Items = make([]models.PlanningItem, 0)
	}
	if payload.Questions == nil {
		payload.Questions = make([]models.PlanningQuestion, 0)
	}
	if payload.Messages == nil {
		payload.Messages = make([]models.PlanningMessage, 0)
	}

	return payload, nil
}

type SummaryPostgres struct {
	db *sql.DB
}

func NewSummaryPostgres(db *sql.DB) *SummaryPostgres {
	return &SummaryPostgres{db: db}
}

func (r *SummaryPostgres) GetByFingerprint(ctx context.Context, userID uuid.UUID, fingerprint string) (*models.AISummary, error) {
	query := `SELECT ` + summaryColumns + ` FROM ai_summaries WHERE user_id = $1 AND fingerprint = $2`

	summary, err := scanSummaryRow(r.db.QueryRowContext(ctx, query, userID, fingerprint))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get ai summary by fingerprint: %w", err)
	}

	return summary, nil
}

func (r *SummaryPostgres) SaveSummary(ctx context.Context, summary *models.AISummary) error {
	content, err := json.Marshal(summary.Content)
	if err != nil {
		return fmt.Errorf("failed to marshal ai summary content: %w", err)
	}

	query := `
        INSERT INTO ai_summaries (user_id, period, range_start, range_end, fingerprint, content, ai_model)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        ON CONFLICT (user_id, fingerprint) DO UPDATE
            SET content = EXCLUDED.content,
                ai_model = EXCLUDED.ai_model,
                created_at = CURRENT_TIMESTAMP
        RETURNING id, created_at
    `
	err = r.db.QueryRowContext(ctx, query,
		summary.UserID,
		summary.Period,
		summary.RangeStart,
		summary.RangeEnd,
		summary.Fingerprint,
		string(content),
		summary.AIModel,
	).Scan(&summary.ID, &summary.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to save ai summary: %w", err)
	}

	return nil
}

func (r *SummaryPostgres) ListSummaries(ctx context.Context, userID uuid.UUID, limit int) (summaries []models.AISummary, err error) {
	query := `SELECT ` + summaryColumns + ` FROM ai_summaries WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2`

	rows, err := r.db.QueryContext(ctx, query, userID, planningListLimit(limit))
	if err != nil {
		return nil, fmt.Errorf("failed to list ai summaries: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	summaries = make([]models.AISummary, 0)
	for rows.Next() {
		summary, scanErr := scanSummaryRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan ai summary: %w", scanErr)
		}
		summaries = append(summaries, *summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return summaries, nil
}

func scanSummaryRow(scanner planningScanner) (*models.AISummary, error) {
	var (
		summary models.AISummary
		content []byte
	)

	err := scanner.Scan(
		&summary.ID,
		&summary.UserID,
		&summary.Period,
		&summary.RangeStart,
		&summary.RangeEnd,
		&summary.Fingerprint,
		&content,
		&summary.AIModel,
		&summary.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if len(content) > 0 {
		if err := json.Unmarshal(content, &summary.Content); err != nil {
			return nil, fmt.Errorf("failed to unmarshal ai summary content: %w", err)
		}
	}

	return &summary, nil
}
