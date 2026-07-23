package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	// 32 bytes of entropy. The token is the only thing standing between a
	// stranger and the shared board, so it has to be far beyond guessable.
	shareTokenBytes = 32
	maxShareLabel   = 120
	// A cap on live links per user, so a compromised session cannot mint an
	// unbounded number of doors that all have to be found and closed later.
	maxActiveShareLinks = 25
	maxShareTTLDays     = 365
)

// ShareOwnerLookup is the sliver of the user repository this service needs.
// Depending on the full interface would mean a share link could, in principle,
// reach password writes; this narrows it to the one read it actually makes.
type ShareOwnerLookup interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error)
}

type ShareService struct {
	repo      repository.ShareRepository
	tasks     Task
	analytics Analytics
	users     ShareOwnerLookup
	logger    *zap.Logger
}

func NewShareService(
	repo repository.ShareRepository,
	tasks Task,
	analytics Analytics,
	users ShareOwnerLookup,
	logger *zap.Logger,
) *ShareService {
	return &ShareService{repo: repo, tasks: tasks, analytics: analytics, users: users, logger: logger}
}

// ShareLinkInput describes a link to mint.
type ShareLinkInput struct {
	Kind     models.ShareKind
	Label    string
	TTLDays  *int
	Period   string
	Timezone string
}

// CreateLink mints a link and returns it with the plaintext token attached.
// That token exists only in this response: only its hash is stored, so it can
// never be recovered or re-displayed later.
func (s *ShareService) CreateLink(ctx context.Context, userID uuid.UUID, in ShareLinkInput) (*models.ShareLink, error) {
	if in.Kind == "" {
		in.Kind = models.ShareBoard
	}
	if !in.Kind.IsValid() {
		return nil, apperrors.ErrInvalidShareKind
	}

	label := strings.TrimSpace(in.Label)
	if len([]rune(label)) > maxShareLabel {
		return nil, apperrors.ErrShareLabelTooLong
	}

	var expiresAt *time.Time
	if in.TTLDays != nil {
		days := *in.TTLDays
		if days <= 0 || days > maxShareTTLDays {
			return nil, apperrors.ErrInvalidShareTTL
		}
		exp := time.Now().UTC().AddDate(0, 0, days)
		expiresAt = &exp
	}

	existing, err := s.repo.ListLinks(ctx, userID)
	if err != nil {
		s.logger.Error("failed to list share links", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("list share links: %w", err)
	}

	now := time.Now().UTC()
	active := 0
	for _, link := range existing {
		if link.Active(now) {
			active++
		}
	}
	if active >= maxActiveShareLinks {
		return nil, apperrors.ErrTooManyShareLinks
	}

	token, hash, err := newShareToken()
	if err != nil {
		s.logger.Error("failed to generate share token", zap.Error(err))
		return nil, fmt.Errorf("generate share token: %w", err)
	}

	link := &models.ShareLink{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: hash,
		Kind:      in.Kind,
		Label:     label,
		ExpiresAt: expiresAt,
	}
	if err := s.repo.CreateLink(ctx, link); err != nil {
		s.logger.Error("failed to create share link", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("create share link: %w", err)
	}

	link.Token = token

	return link, nil
}

func (s *ShareService) ListLinks(ctx context.Context, userID uuid.UUID) ([]models.ShareLink, error) {
	links, err := s.repo.ListLinks(ctx, userID)
	if err != nil {
		s.logger.Error("failed to list share links", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("list share links: %w", err)
	}
	return links, nil
}

func (s *ShareService) RevokeLink(ctx context.Context, userID, linkID uuid.UUID) error {
	if err := s.repo.RevokeLink(ctx, userID, linkID, time.Now().UTC()); err != nil {
		if errors.Is(err, apperrors.ErrShareLinkNotFound) {
			return err
		}
		s.logger.Error("failed to revoke share link", zap.Error(err), zap.String("link_id", linkID.String()))
		return fmt.Errorf("revoke share link: %w", err)
	}
	return nil
}

func (s *ShareService) DeleteLink(ctx context.Context, userID, linkID uuid.UUID) error {
	if err := s.repo.DeleteLink(ctx, userID, linkID); err != nil {
		if errors.Is(err, apperrors.ErrShareLinkNotFound) {
			return err
		}
		s.logger.Error("failed to delete share link", zap.Error(err), zap.String("link_id", linkID.String()))
		return fmt.Errorf("delete share link: %w", err)
	}
	return nil
}

// Resolve turns a public token into the read-only view it grants.
//
// An unknown, revoked or expired token yields the same ErrShareLinkNotFound so
// a caller probing tokens cannot tell "never existed" from "revoked" — the
// difference would confirm that a guessed token was once real.
func (s *ShareService) Resolve(ctx context.Context, token string, q models.AnalyticsQuery) (*models.SharedView, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, apperrors.ErrShareLinkNotFound
	}

	link, err := s.repo.GetByTokenHash(ctx, hashShareToken(token))
	if err != nil {
		if errors.Is(err, apperrors.ErrShareLinkNotFound) {
			return nil, err
		}
		s.logger.Error("failed to resolve share link", zap.Error(err))
		return nil, fmt.Errorf("resolve share link: %w", err)
	}

	now := time.Now().UTC()
	if !link.Active(now) {
		return nil, apperrors.ErrShareLinkNotFound
	}

	owner, err := s.users.GetUserByID(ctx, link.UserID)
	if err != nil {
		s.logger.Error("failed to load share link owner", zap.Error(err), zap.String("link_id", link.ID.String()))
		return nil, fmt.Errorf("load share owner: %w", err)
	}

	view := &models.SharedView{
		Kind:        link.Kind,
		Label:       link.Label,
		OwnerEmail:  owner.Email,
		GeneratedAt: now,
		ExpiresAt:   link.ExpiresAt,
	}

	if link.Kind.IncludesBoard() {
		tasks, err := s.tasks.GetTasks(ctx, link.UserID, models.TaskFilter{})
		if err != nil {
			return nil, fmt.Errorf("load shared tasks: %w", err)
		}
		view.Columns = groupTasksByStatus(tasks)
		view.TaskCount = len(tasks)
	}

	if link.Kind.IncludesDashboard() {
		dashboard, err := s.analytics.Dashboard(ctx, link.UserID, q)
		if err != nil {
			return nil, fmt.Errorf("load shared analytics: %w", err)
		}
		view.Analytics = dashboard
	}

	// Telemetry only: a failed counter update must not deny a legitimate view.
	if err := s.repo.TouchLink(ctx, link.ID, now); err != nil {
		s.logger.Warn("failed to record share link view", zap.Error(err), zap.String("link_id", link.ID.String()))
	}

	return view, nil
}

// groupTasksByStatus lays tasks out in board order, preserving the owner's
// manual ordering within each column.
func groupTasksByStatus(tasks []models.Task) []models.SharedBoardColumn {
	columns := make([]models.SharedBoardColumn, 0, len(models.BoardStatuses))

	for _, status := range models.BoardStatuses {
		bucket := make([]models.Task, 0)
		for _, task := range tasks {
			if task.Status == status {
				bucket = append(bucket, task)
			}
		}
		slices.SortStableFunc(bucket, func(a, b models.Task) int {
			switch {
			case a.Position < b.Position:
				return -1
			case a.Position > b.Position:
				return 1
			default:
				return 0
			}
		})
		public := make([]models.SharedTask, 0, len(bucket))
		for _, task := range bucket {
			public = append(public, models.NewSharedTask(task))
		}

		columns = append(columns, models.SharedBoardColumn{Status: status, Tasks: public})
	}

	return columns
}

// newShareToken returns a URL-safe token and the hash to store for it.
func newShareToken() (token, hash string, err error) {
	buf := make([]byte, shareTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}

	token = base64.RawURLEncoding.EncodeToString(buf)
	return token, hashShareToken(token), nil
}

// hashShareToken derives the stored form of a token.
//
// A plain SHA-256 is right here where it would be wrong for a password: the
// token is 256 bits of machine-generated randomness, so brute force is already
// hopeless and a slow KDF would only add latency to every public page view.
func hashShareToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
