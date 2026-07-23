package service_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// mocks
// ---------------------------------------------------------------------------

type shareRepoMock struct{ mock.Mock }

func (m *shareRepoMock) CreateLink(ctx context.Context, link *models.ShareLink) error {
	args := m.Called(ctx, link)
	return args.Error(0)
}

func (m *shareRepoMock) GetByTokenHash(ctx context.Context, hash string) (*models.ShareLink, error) {
	args := m.Called(ctx, hash)
	link, _ := args.Get(0).(*models.ShareLink)
	return link, args.Error(1)
}

func (m *shareRepoMock) ListLinks(ctx context.Context, userID uuid.UUID) ([]models.ShareLink, error) {
	args := m.Called(ctx, userID)
	links, _ := args.Get(0).([]models.ShareLink)
	return links, args.Error(1)
}

func (m *shareRepoMock) RevokeLink(ctx context.Context, userID, linkID uuid.UUID, at time.Time) error {
	args := m.Called(ctx, userID, linkID, at)
	return args.Error(0)
}

func (m *shareRepoMock) DeleteLink(ctx context.Context, userID, linkID uuid.UUID) error {
	args := m.Called(ctx, userID, linkID)
	return args.Error(0)
}

func (m *shareRepoMock) TouchLink(ctx context.Context, linkID uuid.UUID, at time.Time) error {
	args := m.Called(ctx, linkID, at)
	return args.Error(0)
}

type shareTaskSvcMock struct{ mock.Mock }

func (m *shareTaskSvcMock) GetTasks(ctx context.Context, userID uuid.UUID, f models.TaskFilter) ([]models.Task, error) {
	args := m.Called(ctx, userID, f)
	tasks, _ := args.Get(0).([]models.Task)
	return tasks, args.Error(1)
}

func (m *shareTaskSvcMock) CreateTask(context.Context, uuid.UUID, models.TaskCreate) (*models.Task, error) {
	panic("not used")
}
func (m *shareTaskSvcMock) GetTask(context.Context, uuid.UUID, uuid.UUID) (*models.Task, error) {
	panic("not used")
}
func (m *shareTaskSvcMock) UpdateTask(context.Context, uuid.UUID, uuid.UUID, models.TaskPatch) (*models.Task, error) {
	panic("not used")
}
func (m *shareTaskSvcMock) UpdateTaskStatus(context.Context, string, uuid.UUID, models.TaskStatus, *string) error {
	panic("not used")
}
func (m *shareTaskSvcMock) MoveTask(context.Context, uuid.UUID, uuid.UUID, models.TaskStatus, *uuid.UUID, *uuid.UUID) (*models.Task, error) {
	panic("not used")
}
func (m *shareTaskSvcMock) DeleteTask(context.Context, uuid.UUID, uuid.UUID) error { panic("not used") }
func (m *shareTaskSvcMock) CreateTasksFromPlan(context.Context, uuid.UUID, *uuid.UUID, *models.SprintPlan) ([]models.Task, error) {
	panic("not used")
}

type shareUserRepoMock struct{ mock.Mock }

func (m *shareUserRepoMock) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	args := m.Called(ctx, id)
	u, _ := args.Get(0).(*models.User)
	return u, args.Error(1)
}

type shareFixture struct {
	svc   *service.ShareService
	repo  *shareRepoMock
	tasks *shareTaskSvcMock
	users *shareUserRepoMock
}

func newShareFixture(t *testing.T) *shareFixture {
	t.Helper()

	repo := &shareRepoMock{}
	tasks := &shareTaskSvcMock{}
	users := &shareUserRepoMock{}

	return &shareFixture{
		svc:   service.NewShareService(repo, tasks, nil, users, zap.NewNop()),
		repo:  repo,
		tasks: tasks,
		users: users,
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// creation
// ---------------------------------------------------------------------------

func TestShareService_CreateLink_StoresOnlyTheHash(t *testing.T) {
	f := newShareFixture(t)
	userID := uuid.New()

	var stored *models.ShareLink
	f.repo.On("ListLinks", mock.Anything, userID).Return([]models.ShareLink{}, nil)
	f.repo.On("CreateLink", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		stored = args.Get(1).(*models.ShareLink)
	}).Return(nil)

	link, err := f.svc.CreateLink(context.Background(), userID, service.ShareLinkInput{Kind: models.ShareBoard})
	require.NoError(t, err)

	require.NotEmpty(t, link.Token, "the caller must receive the plaintext token once")
	require.NotNil(t, stored)

	assert.Empty(t, stored.TokenHash[:0], "sanity")
	assert.Equal(t, sha256Hex(link.Token), stored.TokenHash,
		"the database must hold the hash of the token")
	assert.NotContains(t, stored.TokenHash, link.Token,
		"the plaintext token must never be persisted")
}

func TestShareService_CreateLink_MintsADistinctTokenEachTime(t *testing.T) {
	f := newShareFixture(t)
	userID := uuid.New()

	f.repo.On("ListLinks", mock.Anything, userID).Return([]models.ShareLink{}, nil)
	f.repo.On("CreateLink", mock.Anything, mock.Anything).Return(nil)

	seen := make(map[string]struct{})
	for range 50 {
		link, err := f.svc.CreateLink(context.Background(), userID, service.ShareLinkInput{})
		require.NoError(t, err)

		if _, dup := seen[link.Token]; dup {
			t.Fatalf("token collision after %d draws: %s", len(seen), link.Token)
		}
		seen[link.Token] = struct{}{}

		// 32 raw bytes base64url-encode to 43 characters.
		assert.Len(t, link.Token, 43, "token should carry 256 bits of entropy")
	}
}

func TestShareService_CreateLink_DefaultsToABoardLinkThatNeverExpires(t *testing.T) {
	f := newShareFixture(t)
	userID := uuid.New()

	f.repo.On("ListLinks", mock.Anything, userID).Return([]models.ShareLink{}, nil)
	f.repo.On("CreateLink", mock.Anything, mock.Anything).Return(nil)

	link, err := f.svc.CreateLink(context.Background(), userID, service.ShareLinkInput{})
	require.NoError(t, err)

	assert.Equal(t, models.ShareBoard, link.Kind)
	assert.Nil(t, link.ExpiresAt)
}

func TestShareService_CreateLink_RejectsBadInput(t *testing.T) {
	zero, huge, negative := 0, 9999, -3

	cases := []struct {
		name    string
		in      service.ShareLinkInput
		wantErr error
	}{
		{"unknown kind", service.ShareLinkInput{Kind: "EVERYTHING"}, apperrors.ErrInvalidShareKind},
		{"zero ttl", service.ShareLinkInput{TTLDays: &zero}, apperrors.ErrInvalidShareTTL},
		{"negative ttl", service.ShareLinkInput{TTLDays: &negative}, apperrors.ErrInvalidShareTTL},
		{"ttl beyond the cap", service.ShareLinkInput{TTLDays: &huge}, apperrors.ErrInvalidShareTTL},
		{"over-long label", service.ShareLinkInput{Label: string(make([]rune, 121))}, apperrors.ErrShareLabelTooLong},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newShareFixture(t)

			link, err := f.svc.CreateLink(context.Background(), uuid.New(), tc.in)

			require.Error(t, err)
			assert.Nil(t, link)
			assert.ErrorIs(t, err, tc.wantErr)
			f.repo.AssertNotCalled(t, "CreateLink", mock.Anything, mock.Anything)
		})
	}
}

func TestShareService_CreateLink_CapsActiveLinks(t *testing.T) {
	f := newShareFixture(t)
	userID := uuid.New()

	active := make([]models.ShareLink, 25)
	for i := range active {
		active[i] = models.ShareLink{ID: uuid.New()}
	}
	f.repo.On("ListLinks", mock.Anything, userID).Return(active, nil)

	link, err := f.svc.CreateLink(context.Background(), userID, service.ShareLinkInput{})

	require.Error(t, err)
	assert.Nil(t, link)
	assert.ErrorIs(t, err, apperrors.ErrTooManyShareLinks)
	f.repo.AssertNotCalled(t, "CreateLink", mock.Anything, mock.Anything)
}

func TestShareService_CreateLink_RevokedAndExpiredLinksDoNotCountTowardsTheCap(t *testing.T) {
	f := newShareFixture(t)
	userID := uuid.New()

	past := time.Now().UTC().Add(-time.Hour)
	dead := make([]models.ShareLink, 0, 30)
	for range 15 {
		dead = append(dead, models.ShareLink{ID: uuid.New(), RevokedAt: &past})
	}
	for range 15 {
		dead = append(dead, models.ShareLink{ID: uuid.New(), ExpiresAt: &past})
	}

	f.repo.On("ListLinks", mock.Anything, userID).Return(dead, nil)
	f.repo.On("CreateLink", mock.Anything, mock.Anything).Return(nil)

	link, err := f.svc.CreateLink(context.Background(), userID, service.ShareLinkInput{})

	require.NoError(t, err)
	assert.NotEmpty(t, link.Token)
}

// ---------------------------------------------------------------------------
// resolution
// ---------------------------------------------------------------------------

func TestShareService_Resolve_ServesTheOwnersBoard(t *testing.T) {
	f := newShareFixture(t)
	ownerID := uuid.New()
	token := "a-token"

	f.repo.On("GetByTokenHash", mock.Anything, sha256Hex(token)).
		Return(&models.ShareLink{ID: uuid.New(), UserID: ownerID, Kind: models.ShareBoard}, nil)
	f.users.On("GetUserByID", mock.Anything, ownerID).
		Return(&models.User{ID: ownerID, Email: "owner@example.com"}, nil)
	f.tasks.On("GetTasks", mock.Anything, ownerID, mock.Anything).Return([]models.Task{
		{ID: uuid.New(), Title: "second", Status: models.StatusTodo, Position: 2048},
		{ID: uuid.New(), Title: "first", Status: models.StatusTodo, Position: 1024},
		{ID: uuid.New(), Title: "shipped", Status: models.StatusDone, Position: 1024},
	}, nil)
	f.repo.On("TouchLink", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	view, err := f.svc.Resolve(context.Background(), token, models.AnalyticsQuery{})
	require.NoError(t, err)

	assert.Equal(t, "owner@example.com", view.OwnerEmail)
	assert.Equal(t, 3, view.TaskCount)
	assert.Nil(t, view.Analytics, "a board link must not carry analytics")

	byStatus := map[models.TaskStatus][]string{}
	for _, col := range view.Columns {
		for _, task := range col.Tasks {
			byStatus[col.Status] = append(byStatus[col.Status], task.Title)
		}
	}
	assert.Equal(t, []string{"first", "second"}, byStatus[models.StatusTodo],
		"tasks should keep the owner's manual ordering")
	assert.Equal(t, []string{"shipped"}, byStatus[models.StatusDone])
}

func TestShareService_Resolve_RefusesDeadLinks(t *testing.T) {
	past := time.Now().UTC().Add(-time.Minute)

	cases := map[string]*models.ShareLink{
		"revoked": {ID: uuid.New(), UserID: uuid.New(), Kind: models.ShareBoard, RevokedAt: &past},
		"expired": {ID: uuid.New(), UserID: uuid.New(), Kind: models.ShareBoard, ExpiresAt: &past},
	}

	for name, link := range cases {
		t.Run(name, func(t *testing.T) {
			f := newShareFixture(t)
			f.repo.On("GetByTokenHash", mock.Anything, mock.Anything).Return(link, nil)

			view, err := f.svc.Resolve(context.Background(), "token", models.AnalyticsQuery{})

			require.Error(t, err)
			assert.Nil(t, view)
			assert.ErrorIs(t, err, apperrors.ErrShareLinkNotFound,
				"a dead link must be indistinguishable from one that never existed")
			f.tasks.AssertNotCalled(t, "GetTasks", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

func TestShareService_Resolve_RejectsAnUnknownOrBlankToken(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		f := newShareFixture(t)
		f.repo.On("GetByTokenHash", mock.Anything, mock.Anything).
			Return(nil, apperrors.ErrShareLinkNotFound)

		_, err := f.svc.Resolve(context.Background(), "nope", models.AnalyticsQuery{})
		assert.ErrorIs(t, err, apperrors.ErrShareLinkNotFound)
	})

	t.Run("blank never reaches the database", func(t *testing.T) {
		f := newShareFixture(t)

		_, err := f.svc.Resolve(context.Background(), "   ", models.AnalyticsQuery{})

		assert.ErrorIs(t, err, apperrors.ErrShareLinkNotFound)
		f.repo.AssertNotCalled(t, "GetByTokenHash", mock.Anything, mock.Anything)
	})
}

func TestShareService_Resolve_LooksUpByHashNotByPlaintext(t *testing.T) {
	f := newShareFixture(t)
	token := "the-secret-token"

	// Only a call carrying the hash is stubbed; a lookup by plaintext would
	// find no matching expectation and fail the test.
	f.repo.On("GetByTokenHash", mock.Anything, sha256Hex(token)).
		Return(nil, apperrors.ErrShareLinkNotFound)

	_, err := f.svc.Resolve(context.Background(), token, models.AnalyticsQuery{})

	assert.ErrorIs(t, err, apperrors.ErrShareLinkNotFound)
	f.repo.AssertCalled(t, "GetByTokenHash", mock.Anything, sha256Hex(token))
}

func TestShareService_Resolve_SurvivesAFailedViewCounter(t *testing.T) {
	f := newShareFixture(t)
	ownerID := uuid.New()

	f.repo.On("GetByTokenHash", mock.Anything, mock.Anything).
		Return(&models.ShareLink{ID: uuid.New(), UserID: ownerID, Kind: models.ShareBoard}, nil)
	f.users.On("GetUserByID", mock.Anything, ownerID).
		Return(&models.User{ID: ownerID, Email: "owner@example.com"}, nil)
	f.tasks.On("GetTasks", mock.Anything, ownerID, mock.Anything).Return([]models.Task{}, nil)
	f.repo.On("TouchLink", mock.Anything, mock.Anything, mock.Anything).
		Return(errors.New("counter update failed"))

	view, err := f.svc.Resolve(context.Background(), "token", models.AnalyticsQuery{})

	require.NoError(t, err, "telemetry must not deny a legitimate view")
	assert.NotNil(t, view)
}

func TestShareService_Resolve_PublicTasksCarryNoOwnerIdentifiers(t *testing.T) {
	f := newShareFixture(t)
	ownerID := uuid.New()
	sprintID := uuid.New()

	f.repo.On("GetByTokenHash", mock.Anything, mock.Anything).
		Return(&models.ShareLink{ID: uuid.New(), UserID: ownerID, Kind: models.ShareBoard}, nil)
	f.users.On("GetUserByID", mock.Anything, ownerID).
		Return(&models.User{ID: ownerID, Email: "owner@example.com"}, nil)
	f.tasks.On("GetTasks", mock.Anything, ownerID, mock.Anything).Return([]models.Task{
		{ID: uuid.New(), UserID: ownerID, SprintID: &sprintID, Title: "task", Status: models.StatusTodo},
	}, nil)
	f.repo.On("TouchLink", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	view, err := f.svc.Resolve(context.Background(), "token", models.AnalyticsQuery{})
	require.NoError(t, err)

	// SharedTask has no UserID field at all — this asserts the projection is
	// the type in the payload, so the compiler keeps the guarantee.
	require.Len(t, view.Columns[0].Tasks, 1)
	var _ models.SharedTask = view.Columns[0].Tasks[0]
	assert.Equal(t, "task", view.Columns[0].Tasks[0].Title)
}

// ---------------------------------------------------------------------------
// revocation
// ---------------------------------------------------------------------------

func TestShareService_RevokeAndDelete_PropagateNotFound(t *testing.T) {
	userID, linkID := uuid.New(), uuid.New()

	t.Run("revoke", func(t *testing.T) {
		f := newShareFixture(t)
		f.repo.On("RevokeLink", mock.Anything, userID, linkID, mock.Anything).
			Return(apperrors.ErrShareLinkNotFound)

		err := f.svc.RevokeLink(context.Background(), userID, linkID)
		assert.ErrorIs(t, err, apperrors.ErrShareLinkNotFound)
	})

	t.Run("delete", func(t *testing.T) {
		f := newShareFixture(t)
		f.repo.On("DeleteLink", mock.Anything, userID, linkID).
			Return(apperrors.ErrShareLinkNotFound)

		err := f.svc.DeleteLink(context.Background(), userID, linkID)
		assert.ErrorIs(t, err, apperrors.ErrShareLinkNotFound)
	})
}
