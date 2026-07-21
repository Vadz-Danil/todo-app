package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"todo-app/internal/apperrors"
	"todo-app/internal/models"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

type UserRepository interface {
	CreateUser(ctx context.Context, user *models.User) error
	GetUserByEmail(ctx context.Context, email string) (*models.User, error)
	FindOrCreateGoogleUser(ctx context.Context, email, googleID string) (*models.User, error)
}

type UserPostgres struct {
	db *sql.DB
}

func NewUserPostgres(db *sql.DB) *UserPostgres {
	return &UserPostgres{db: db}
}

func (r *UserPostgres) CreateUser(ctx context.Context, user *models.User) error {
	query := `INSERT INTO users (id, email, password_hash, created_at) VALUES ($1, $2, $3, $4)`
	_, err := r.db.ExecContext(ctx, query, user.ID, user.Email, user.PasswordHash, user.CreatedAt)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return apperrors.ErrUserAlreadyExists
		}
		return err
	}
	return nil
}

func (r *UserPostgres) GetUserByEmail(ctx context.Context, email string) (*models.User, error) {
	query := `
        SELECT id, email, password_hash, google_id, created_at 
        FROM users 
        WHERE email = $1
    `
	user := &models.User{}

	err := r.db.QueryRowContext(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.GoogleID,
		&user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperrors.ErrUserNotFound
		}
		return nil, err
	}

	return user, nil
}

func (r *UserPostgres) FindOrCreateGoogleUser(ctx context.Context, email, googleID string) (*models.User, error) {
	query := `SELECT id, email, password_hash, google_id, created_at FROM users WHERE google_id = $1 OR email = $2`
	user := &models.User{}
	err := r.db.QueryRowContext(ctx, query, googleID, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.GoogleID, &user.CreatedAt)

	if err == nil {
		if user.GoogleID == nil {
			updateQuery := `UPDATE users SET google_id = $1 WHERE id = $2`
			r.db.ExecContext(ctx, updateQuery, googleID, user.ID)
			user.GoogleID = &googleID
		}
		return user, nil
	}

	newUser := &models.User{
		ID:        uuid.New(),
		Email:     email,
		GoogleID:  &googleID,
		CreatedAt: time.Now(),
	}
	insertQuery := `INSERT INTO users (id, email, google_id, created_at) VALUES ($1, $2, $3, $4)`
	_, err = r.db.ExecContext(ctx, insertQuery, newUser.ID, newUser.Email, newUser.GoogleID, newUser.CreatedAt)
	if err != nil {
		return nil, err
	}
	return newUser, nil
}
