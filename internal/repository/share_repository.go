package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"

	"github.com/google/uuid"
)

const shareLinkColumns = `id, user_id, token_hash, kind, label, expires_at,
       revoked_at, view_count, last_viewed_at, created_at`

type SharePostgres struct {
	db *sql.DB
}

func NewSharePostgres(db *sql.DB) *SharePostgres {
	return &SharePostgres{db: db}
}

func (r *SharePostgres) CreateLink(ctx context.Context, link *models.ShareLink) error {
	if link.ID == uuid.Nil {
		link.ID = uuid.New()
	}

	query := `
        INSERT INTO share_links (id, user_id, token_hash, kind, label, expires_at)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING created_at
    `
	err := r.db.QueryRowContext(ctx, query,
		link.ID,
		link.UserID,
		link.TokenHash,
		string(link.Kind),
		link.Label,
		link.ExpiresAt,
	).Scan(&link.CreatedAt)
	if err != nil {
		return fmt.Errorf("create share link: %w", err)
	}

	return nil
}

// GetByTokenHash resolves a public link. It deliberately takes only the hash:
// the caller never gets to search by user, so a wrong token cannot be turned
// into a listing of somebody's links.
func (r *SharePostgres) GetByTokenHash(ctx context.Context, tokenHash string) (*models.ShareLink, error) {
	query := `SELECT ` + shareLinkColumns + ` FROM share_links WHERE token_hash = $1`

	link, err := scanShareLink(r.db.QueryRowContext(ctx, query, tokenHash))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.ErrShareLinkNotFound
		}
		return nil, fmt.Errorf("get share link by token: %w", err)
	}

	return link, nil
}

func (r *SharePostgres) ListLinks(ctx context.Context, userID uuid.UUID) ([]models.ShareLink, error) {
	query := `SELECT ` + shareLinkColumns + `
              FROM share_links
              WHERE user_id = $1
              ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("list share links: %w", err)
	}
	defer func() { _ = rows.Close() }()

	links := make([]models.ShareLink, 0)
	for rows.Next() {
		link, err := scanShareLink(rows)
		if err != nil {
			return nil, fmt.Errorf("scan share link: %w", err)
		}
		links = append(links, *link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate share links: %w", err)
	}

	return links, nil
}

// RevokeLink marks a link dead. Revoking an already-revoked link keeps the
// original timestamp so the audit trail is not rewritten by a repeat click.
func (r *SharePostgres) RevokeLink(ctx context.Context, userID, linkID uuid.UUID, at time.Time) error {
	query := `
        UPDATE share_links
        SET revoked_at = COALESCE(revoked_at, $3)
        WHERE id = $1 AND user_id = $2
    `
	res, err := r.db.ExecContext(ctx, query, linkID, userID, at)
	if err != nil {
		return fmt.Errorf("revoke share link: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke share link: %w", err)
	}
	if affected == 0 {
		return apperrors.ErrShareLinkNotFound
	}

	return nil
}

func (r *SharePostgres) DeleteLink(ctx context.Context, userID, linkID uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM share_links WHERE id = $1 AND user_id = $2`, linkID, userID)
	if err != nil {
		return fmt.Errorf("delete share link: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete share link: %w", err)
	}
	if affected == 0 {
		return apperrors.ErrShareLinkNotFound
	}

	return nil
}

// TouchLink records a view. It is best-effort by design: the counter is
// telemetry, so a failure here must never deny someone the page they are
// entitled to see.
func (r *SharePostgres) TouchLink(ctx context.Context, linkID uuid.UUID, at time.Time) error {
	query := `
        UPDATE share_links
        SET view_count = view_count + 1, last_viewed_at = $2
        WHERE id = $1
    `
	if _, err := r.db.ExecContext(ctx, query, linkID, at); err != nil {
		return fmt.Errorf("touch share link: %w", err)
	}
	return nil
}

func scanShareLink(row interface{ Scan(...any) error }) (*models.ShareLink, error) {
	var (
		link  models.ShareLink
		kind  string
		label sql.NullString
	)

	err := row.Scan(
		&link.ID,
		&link.UserID,
		&link.TokenHash,
		&kind,
		&label,
		&link.ExpiresAt,
		&link.RevokedAt,
		&link.ViewCount,
		&link.LastViewedAt,
		&link.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	link.Kind = models.ShareKind(kind)
	link.Label = label.String

	return &link, nil
}
