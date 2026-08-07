package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type subRepoMock struct{ mock.Mock }

func (m *subRepoMock) ListByTask(ctx context.Context, userID, taskID uuid.UUID) ([]models.Subtask, error) {
	args := m.Called(ctx, userID, taskID)
	items, _ := args.Get(0).([]models.Subtask)
	return items, args.Error(1)
}

func (m *subRepoMock) ProgressByTasks(ctx context.Context, userID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]models.SubtaskProgress, error) {
	args := m.Called(ctx, userID, ids)
	p, _ := args.Get(0).(map[uuid.UUID]models.SubtaskProgress)
	return p, args.Error(1)
}

func (m *subRepoMock) NextPosition(ctx context.Context, userID, taskID uuid.UUID) (float64, error) {
	args := m.Called(ctx, userID, taskID)
	return args.Get(0).(float64), args.Error(1)
}

func (m *subRepoMock) Create(ctx context.Context, item *models.Subtask) error {
	return m.Called(ctx, item).Error(0)
}

func (m *subRepoMock) Update(ctx context.Context, userID, subtaskID uuid.UUID, patch models.SubtaskPatch) (*models.Subtask, error) {
	args := m.Called(ctx, userID, subtaskID, patch)
	item, _ := args.Get(0).(*models.Subtask)
	return item, args.Error(1)
}

func (m *subRepoMock) Delete(ctx context.Context, userID, subtaskID uuid.UUID) error {
	return m.Called(ctx, userID, subtaskID).Error(0)
}

// subTaskGuardMock stands in for the task service's ownership check.
type subTaskGuardMock struct{ mock.Mock }

func (m *subTaskGuardMock) GetTask(ctx context.Context, userID, taskID uuid.UUID) (*models.Task, error) {
	args := m.Called(ctx, userID, taskID)
	t, _ := args.Get(0).(*models.Task)
	return t, args.Error(1)
}

type subFixture struct {
	svc   *service.SubtaskService
	repo  *subRepoMock
	guard *subTaskGuardMock
}

func newSubFixture(t *testing.T) *subFixture {
	t.Helper()
	repo := &subRepoMock{}
	guard := &subTaskGuardMock{}
	return &subFixture{
		svc:   service.NewSubtaskService(repo, guard, zap.NewNop()),
		repo:  repo,
		guard: guard,
	}
}

func TestSubtaskService_Create_AppendsToAnOwnedTask(t *testing.T) {
	f := newSubFixture(t)
	userID, taskID := uuid.New(), uuid.New()

	f.guard.On("GetTask", mock.Anything, userID, taskID).Return(&models.Task{ID: taskID}, nil)
	f.repo.On("NextPosition", mock.Anything, userID, taskID).Return(2048.0, nil)

	var stored *models.Subtask
	f.repo.On("Create", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		stored = args.Get(1).(*models.Subtask)
	}).Return(nil)

	item, err := f.svc.Create(context.Background(), userID, taskID, models.SubtaskCreate{Title: "  Write tests  "})
	require.NoError(t, err)

	assert.Equal(t, "Write tests", item.Title, "the title should be trimmed")
	assert.Equal(t, taskID, item.TaskID)
	assert.Equal(t, userID, stored.UserID, "user_id must be stamped for the ownership guard")
	assert.Equal(t, 2048.0, item.Position)
	assert.False(t, item.Done)
}

func TestSubtaskService_Create_RejectsABlankTitleBeforeTouchingTheRepo(t *testing.T) {
	f := newSubFixture(t)

	item, err := f.svc.Create(context.Background(), uuid.New(), uuid.New(), models.SubtaskCreate{Title: "   "})

	require.Error(t, err)
	assert.Nil(t, item)
	assert.ErrorIs(t, err, apperrors.ErrEmptySubtaskTitle)
	f.guard.AssertNotCalled(t, "GetTask", mock.Anything, mock.Anything, mock.Anything)
	f.repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestSubtaskService_Create_RejectsAnOverlongTitle(t *testing.T) {
	f := newSubFixture(t)

	_, err := f.svc.Create(context.Background(), uuid.New(), uuid.New(),
		models.SubtaskCreate{Title: strings.Repeat("a", 501)})

	assert.ErrorIs(t, err, apperrors.ErrSubtaskTitleTooLong)
	f.repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestSubtaskService_Create_RefusesAForeignTask(t *testing.T) {
	f := newSubFixture(t)
	userID, taskID := uuid.New(), uuid.New()

	// The task guard reports the task as not found for this user.
	f.guard.On("GetTask", mock.Anything, userID, taskID).Return(nil, apperrors.ErrTaskNotFound)

	item, err := f.svc.Create(context.Background(), userID, taskID, models.SubtaskCreate{Title: "sneak in"})

	require.Error(t, err)
	assert.Nil(t, item)
	assert.ErrorIs(t, err, apperrors.ErrTaskNotFound)
	f.repo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestSubtaskService_List_ChecksOwnershipFirst(t *testing.T) {
	f := newSubFixture(t)
	userID, taskID := uuid.New(), uuid.New()

	f.guard.On("GetTask", mock.Anything, userID, taskID).Return(nil, apperrors.ErrTaskNotFound)

	items, err := f.svc.List(context.Background(), userID, taskID)

	assert.ErrorIs(t, err, apperrors.ErrTaskNotFound)
	assert.Nil(t, items)
	f.repo.AssertNotCalled(t, "ListByTask", mock.Anything, mock.Anything, mock.Anything)
}

func TestSubtaskService_Update_TrimsTitleAndForwardsDone(t *testing.T) {
	f := newSubFixture(t)
	userID, subtaskID := uuid.New(), uuid.New()
	done := true

	var gotPatch models.SubtaskPatch
	f.repo.On("Update", mock.Anything, userID, subtaskID, mock.Anything).
		Run(func(args mock.Arguments) { gotPatch = args.Get(3).(models.SubtaskPatch) }).
		Return(&models.Subtask{ID: subtaskID, Done: true}, nil)

	title := "  Done item  "
	_, err := f.svc.Update(context.Background(), userID, subtaskID, models.SubtaskPatch{Title: &title, Done: &done})
	require.NoError(t, err)

	require.NotNil(t, gotPatch.Title)
	assert.Equal(t, "Done item", *gotPatch.Title, "the title should reach the repo trimmed")
	require.NotNil(t, gotPatch.Done)
	assert.True(t, *gotPatch.Done)
}

func TestSubtaskService_Update_RejectsABlankTitle(t *testing.T) {
	f := newSubFixture(t)
	blank := "   "

	_, err := f.svc.Update(context.Background(), uuid.New(), uuid.New(), models.SubtaskPatch{Title: &blank})

	assert.ErrorIs(t, err, apperrors.ErrEmptySubtaskTitle)
	f.repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestSubtaskService_UpdateAndDelete_PropagateNotFound(t *testing.T) {
	userID, id := uuid.New(), uuid.New()
	done := true

	t.Run("update", func(t *testing.T) {
		f := newSubFixture(t)
		f.repo.On("Update", mock.Anything, userID, id, mock.Anything).
			Return(nil, apperrors.ErrSubtaskNotFound)

		_, err := f.svc.Update(context.Background(), userID, id, models.SubtaskPatch{Done: &done})
		assert.ErrorIs(t, err, apperrors.ErrSubtaskNotFound)
	})

	t.Run("delete", func(t *testing.T) {
		f := newSubFixture(t)
		f.repo.On("Delete", mock.Anything, userID, id).Return(apperrors.ErrSubtaskNotFound)

		err := f.svc.Delete(context.Background(), userID, id)
		assert.ErrorIs(t, err, apperrors.ErrSubtaskNotFound)
	})
}

func TestSubtaskService_Progress_PassesThrough(t *testing.T) {
	f := newSubFixture(t)
	userID := uuid.New()
	t1 := uuid.New()

	want := map[uuid.UUID]models.SubtaskProgress{t1: {Done: 2, Total: 3}}
	f.repo.On("ProgressByTasks", mock.Anything, userID, []uuid.UUID{t1}).Return(want, nil)

	got, err := f.svc.Progress(context.Background(), userID, []uuid.UUID{t1})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestSubtaskService_Create_WrapsUnexpectedRepoErrors(t *testing.T) {
	f := newSubFixture(t)
	userID, taskID := uuid.New(), uuid.New()

	f.guard.On("GetTask", mock.Anything, userID, taskID).Return(&models.Task{ID: taskID}, nil)
	f.repo.On("NextPosition", mock.Anything, userID, taskID).Return(1024.0, nil)
	f.repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("db down"))

	_, err := f.svc.Create(context.Background(), userID, taskID, models.SubtaskCreate{Title: "x"})

	require.Error(t, err)
	// An infrastructure error must not masquerade as a domain error.
	assert.NotErrorIs(t, err, apperrors.ErrTaskNotFound)
	assert.NotErrorIs(t, err, apperrors.ErrEmptySubtaskTitle)
}
