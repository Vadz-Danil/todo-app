package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"

	"github.com/google/uuid"
)

const exportTargetColumns = `id, user_id, name, url, secret, headers, enabled,
       last_status, last_error, last_sent_at, created_at, updated_at`

const exportDeliveryColumns = `id, user_id, target_id, url, kind, status, status_code,
       attempts, duration_ms, payload_size, error, created_at`

const (
	defaultDeliveryLimit = 50
	maxDeliveryLimit     = 200
)

type ExportPostgres struct {
	db *sql.DB
}

func NewExportPostgres(db *sql.DB) *ExportPostgres {
	return &ExportPostgres{db: db}
}

func (r *ExportPostgres) CreateTarget(ctx context.Context, target *models.ExportTarget) error {
	if target.ID == uuid.Nil {
		target.ID = uuid.New()
	}

	headers, err := marshalExportHeaders(target.Headers)
	if err != nil {
		return err
	}

	query := `
        INSERT INTO export_targets (id, user_id, name, url, secret, headers, enabled)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING created_at, updated_at
    `
	err = r.db.QueryRowContext(ctx, query,
		target.ID,
		target.UserID,
		target.Name,
		target.URL,
		exportNullString(target.Secret),
		headers,
		target.Enabled,
	).Scan(&target.CreatedAt, &target.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to create export target: %w", err)
	}

	if target.Headers == nil {
		target.Headers = make(map[string]string)
	}
	target.HasSecret = strings.TrimSpace(target.Secret) != ""

	return nil
}

func (r *ExportPostgres) GetTarget(ctx context.Context, userID, targetID uuid.UUID) (*models.ExportTarget, error) {
	query := `SELECT ` + exportTargetColumns + ` FROM export_targets WHERE id = $1 AND user_id = $2`

	target, err := scanExportTarget(r.db.QueryRowContext(ctx, query, targetID, userID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.ErrExportTargetNotFound
		}
		return nil, fmt.Errorf("failed to get export target: %w", err)
	}

	return target, nil
}

func (r *ExportPostgres) ListTargets(ctx context.Context, userID uuid.UUID) (targets []models.ExportTarget, err error) {
	query := `SELECT ` + exportTargetColumns + ` FROM export_targets WHERE user_id = $1 ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list export targets: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	targets = make([]models.ExportTarget, 0)
	for rows.Next() {
		target, scanErr := scanExportTarget(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan export target: %w", scanErr)
		}
		targets = append(targets, *target)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return targets, nil
}

func (r *ExportPostgres) UpdateTarget(ctx context.Context, target *models.ExportTarget) error {
	headers, err := marshalExportHeaders(target.Headers)
	if err != nil {
		return err
	}

	query := `
        UPDATE export_targets
        SET name = $1, url = $2, secret = $3, headers = $4, enabled = $5, updated_at = CURRENT_TIMESTAMP
        WHERE id = $6 AND user_id = $7
        RETURNING created_at, updated_at
    `
	err = r.db.QueryRowContext(ctx, query,
		target.Name,
		target.URL,
		exportNullString(target.Secret),
		headers,
		target.Enabled,
		target.ID,
		target.UserID,
	).Scan(&target.CreatedAt, &target.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperrors.ErrExportTargetNotFound
		}
		return fmt.Errorf("failed to update export target: %w", err)
	}

	if target.Headers == nil {
		target.Headers = make(map[string]string)
	}
	target.HasSecret = strings.TrimSpace(target.Secret) != ""

	return nil
}

func (r *ExportPostgres) DeleteTarget(ctx context.Context, userID, targetID uuid.UUID) error {
	query := `DELETE FROM export_targets WHERE id = $1 AND user_id = $2`

	res, err := r.db.ExecContext(ctx, query, targetID, userID)
	if err != nil {
		return fmt.Errorf("failed to delete export target: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return apperrors.ErrExportTargetNotFound
	}

	return nil
}

// TouchTarget records the outcome of a delivery attempt. A target that was
// deleted mid-flight is not an error: the push itself already succeeded.
func (r *ExportPostgres) TouchTarget(ctx context.Context, userID, targetID uuid.UUID, statusCode *int, errMsg *string, sentAt time.Time) error {
	query := `
        UPDATE export_targets
        SET last_status = $1, last_error = $2, last_sent_at = $3, updated_at = CURRENT_TIMESTAMP
        WHERE id = $4 AND user_id = $5
    `
	if _, err := r.db.ExecContext(ctx, query, statusCode, errMsg, sentAt, targetID, userID); err != nil {
		return fmt.Errorf("failed to touch export target: %w", err)
	}

	return nil
}

func (r *ExportPostgres) RecordDelivery(ctx context.Context, delivery *models.ExportDelivery) error {
	if delivery.ID == uuid.Nil {
		delivery.ID = uuid.New()
	}

	query := `
        INSERT INTO export_deliveries (id, user_id, target_id, url, kind, status, status_code,
                                       attempts, duration_ms, payload_size, error)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        RETURNING created_at
    `
	err := r.db.QueryRowContext(ctx, query,
		delivery.ID,
		delivery.UserID,
		delivery.TargetID,
		delivery.URL,
		string(delivery.Kind),
		delivery.Status,
		delivery.StatusCode,
		delivery.Attempts,
		delivery.DurationMS,
		delivery.PayloadSize,
		delivery.Error,
	).Scan(&delivery.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to record export delivery: %w", err)
	}

	return nil
}

func (r *ExportPostgres) ListDeliveries(ctx context.Context, userID uuid.UUID, limit int) (deliveries []models.ExportDelivery, err error) {
	if limit <= 0 {
		limit = defaultDeliveryLimit
	}
	if limit > maxDeliveryLimit {
		limit = maxDeliveryLimit
	}

	query := `SELECT ` + exportDeliveryColumns + `
        FROM export_deliveries
        WHERE user_id = $1
        ORDER BY created_at DESC
        LIMIT $2`

	rows, err := r.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list export deliveries: %w", err)
	}

	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("failed to close rows: %w", closeErr))
		}
	}()

	deliveries = make([]models.ExportDelivery, 0)
	for rows.Next() {
		var (
			d           models.ExportDelivery
			targetID    uuid.NullUUID
			statusCode  sql.NullInt64
			deliveryErr sql.NullString
		)
		if err := rows.Scan(
			&d.ID,
			&d.UserID,
			&targetID,
			&d.URL,
			&d.Kind,
			&d.Status,
			&statusCode,
			&d.Attempts,
			&d.DurationMS,
			&d.PayloadSize,
			&deliveryErr,
			&d.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan export delivery: %w", err)
		}

		if targetID.Valid {
			id := targetID.UUID
			d.TargetID = &id
		}
		if statusCode.Valid {
			code := int(statusCode.Int64)
			d.StatusCode = &code
		}
		if deliveryErr.Valid {
			msg := deliveryErr.String
			d.Error = &msg
		}

		deliveries = append(deliveries, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during rows iteration: %w", err)
	}

	return deliveries, nil
}

type exportRowScanner interface {
	Scan(dest ...any) error
}

func scanExportTarget(scanner exportRowScanner) (*models.ExportTarget, error) {
	var (
		target     models.ExportTarget
		secret     sql.NullString
		headers    []byte
		lastStatus sql.NullInt64
		lastError  sql.NullString
		lastSentAt sql.NullTime
	)

	if err := scanner.Scan(
		&target.ID,
		&target.UserID,
		&target.Name,
		&target.URL,
		&secret,
		&headers,
		&target.Enabled,
		&lastStatus,
		&lastError,
		&lastSentAt,
		&target.CreatedAt,
		&target.UpdatedAt,
	); err != nil {
		return nil, err
	}

	parsedHeaders, err := unmarshalExportHeaders(headers)
	if err != nil {
		return nil, err
	}
	target.Headers = parsedHeaders

	// The service needs the raw secret for HMAC signing; the JSON tag keeps it
	// away from clients, which see only HasSecret.
	target.Secret = secret.String
	target.HasSecret = strings.TrimSpace(secret.String) != ""

	if lastStatus.Valid {
		code := int(lastStatus.Int64)
		target.LastStatus = &code
	}
	if lastError.Valid {
		msg := lastError.String
		target.LastError = &msg
	}
	if lastSentAt.Valid {
		sentAt := lastSentAt.Time
		target.LastSentAt = &sentAt
	}

	return &target, nil
}

func marshalExportHeaders(headers map[string]string) ([]byte, error) {
	if len(headers) == 0 {
		return []byte("{}"), nil
	}
	raw, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal export target headers: %w", err)
	}
	return raw, nil
}

// unmarshalExportHeaders always yields a non-nil map so the target serialises
// as {} rather than null.
func unmarshalExportHeaders(raw []byte) (map[string]string, error) {
	headers := make(map[string]string)
	if len(raw) == 0 {
		return headers, nil
	}
	if err := json.Unmarshal(raw, &headers); err != nil {
		return nil, fmt.Errorf("failed to unmarshal export target headers: %w", err)
	}
	if headers == nil {
		headers = make(map[string]string)
	}
	return headers, nil
}

func exportNullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: strings.TrimSpace(value) != ""}
}
