package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/repository"
	"todo-app/internal/service"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ---------------------------------------------------------------------------
// mock repository
// ---------------------------------------------------------------------------

type tsMockRepo struct {
	mock.Mock
}

var _ repository.TaskRepository = (*tsMockRepo)(nil)

func (m *tsMockRepo) CreateTask(ctx context.Context, task *models.Task) error {
	return m.Called(ctx, task).Error(0)
}

func (m *tsMockRepo) CreateTasksBulk(ctx context.Context, tasks []models.Task) error {
	return m.Called(ctx, tasks).Error(0)
}

func (m *tsMockRepo) GetTaskByID(ctx context.Context, userID, taskID uuid.UUID) (*models.Task, error) {
	args := m.Called(ctx, userID, taskID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Task), args.Error(1)
}

func (m *tsMockRepo) ListTasks(ctx context.Context, userID uuid.UUID, filter models.TaskFilter) ([]models.Task, error) {
	args := m.Called(ctx, userID, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Task), args.Error(1)
}

func (m *tsMockRepo) UpdateTask(ctx context.Context, userID, taskID uuid.UUID, patch models.TaskPatch) (*models.Task, error) {
	args := m.Called(ctx, userID, taskID, patch)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Task), args.Error(1)
}

func (m *tsMockRepo) DeleteTask(ctx context.Context, userID, taskID uuid.UUID) error {
	return m.Called(ctx, userID, taskID).Error(0)
}

func (m *tsMockRepo) AppendPosition(ctx context.Context, userID uuid.UUID, status models.TaskStatus) (float64, error) {
	args := m.Called(ctx, userID, status)
	return args.Get(0).(float64), args.Error(1)
}

func (m *tsMockRepo) PositionBetween(ctx context.Context, userID uuid.UUID, status models.TaskStatus, afterID, beforeID *uuid.UUID) (float64, error) {
	args := m.Called(ctx, userID, status, afterID, beforeID)
	return args.Get(0).(float64), args.Error(1)
}

func (m *tsMockRepo) RecordStatusChange(ctx context.Context, change *models.StatusChange) error {
	return m.Called(ctx, change).Error(0)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func tsPtr[T any](v T) *T { return &v }

func tsNewSvc(t *testing.T) (*service.TaskService, *tsMockRepo) {
	t.Helper()
	repo := new(tsMockRepo)
	t.Cleanup(func() { repo.AssertExpectations(t) })
	return service.NewTaskService(repo, nil, zap.NewNop()), repo
}

// tsExpectCreate stubs repo.CreateTask and captures the row the service builds.
func tsExpectCreate(repo *tsMockRepo, created **models.Task) *mock.Call {
	return repo.On("CreateTask", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { *created = args.Get(1).(*models.Task) }).
		Return(nil)
}

// tsExpectUpdate stubs repo.UpdateTask, captures the patch the service assembled
// and echoes back a task with that patch applied, the way a real repo would.
func tsExpectUpdate(repo *tsMockRepo, current models.Task, captured *models.TaskPatch) *models.Task {
	echoed := new(models.Task)
	repo.On("UpdateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			patch := args.Get(3).(models.TaskPatch)
			*captured = patch
			*echoed = tsApplyPatch(current, patch)
		}).
		Return(echoed, nil)
	return echoed
}

// tsApplyPatch mirrors the repository's UPDATE for the columns these tests assert
// on, so the returned task reflects the patch the service actually sent.
func tsApplyPatch(task models.Task, patch models.TaskPatch) models.Task {
	if patch.Title != nil {
		task.Title = *patch.Title
	}
	if patch.Status != nil {
		task.Status = *patch.Status
	}
	if patch.Priority != nil {
		task.Priority = *patch.Priority
	}
	if patch.Reviewer != nil {
		task.Reviewer = *patch.Reviewer
	}
	if patch.Description != nil {
		task.Description = *patch.Description
	}
	if patch.EstimateHours != nil {
		task.EstimateHours = *patch.EstimateHours
	}
	if patch.BufferHours != nil {
		task.BufferHours = *patch.BufferHours
	}
	if patch.SpentHours != nil {
		task.SpentHours = *patch.SpentHours
	}
	if patch.Position != nil {
		task.Position = *patch.Position
	}
	if patch.StartedAt != nil {
		task.StartedAt = *patch.StartedAt
	}
	if patch.CompletedAt != nil {
		task.CompletedAt = *patch.CompletedAt
	}
	return task
}

// tsWindow brackets a call so stamped timestamps can be range-asserted.
func tsWindow() (time.Time, func() time.Time) {
	start := time.Now().UTC().Add(-time.Second)
	return start, func() time.Time { return time.Now().UTC().Add(time.Second) }
}

// ---------------------------------------------------------------------------
// title validation
// ---------------------------------------------------------------------------

func TestTaskService_TitleValidation(t *testing.T) {
	userID := uuid.New()

	t.Run("rejected titles never reach the repository", func(t *testing.T) {
		cases := []struct {
			name    string
			title   string
			wantErr error
		}{
			{"blank", "", apperrors.ErrEmptyTaskTitle},
			{"whitespace only", "   ", apperrors.ErrEmptyTaskTitle},
			{"tabs and newlines only", "\t\n  \r\n", apperrors.ErrEmptyTaskTitle},
			{"256 runes", strings.Repeat("a", 256), apperrors.ErrTaskTitleTooLong},
			{"256 multi-byte runes", strings.Repeat("д", 256), apperrors.ErrTaskTitleTooLong},
			{"trims down to blank around padding", " " + "  " + "\t", apperrors.ErrEmptyTaskTitle},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, repo := tsNewSvc(t)

				task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{Title: tc.title})

				assert.Nil(t, task)
				assert.ErrorIs(t, err, tc.wantErr)
				repo.AssertNotCalled(t, "AppendPosition", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "CreateTask", mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("accepted titles are trimmed before they reach the repository", func(t *testing.T) {
		cases := []struct {
			name  string
			title string
			want  string
		}{
			{"surrounding spaces", "  Ship the board  ", "Ship the board"},
			{"newlines and tabs", "\n\tShip the board\t\n", "Ship the board"},
			{"inner spacing preserved", "  Ship   the board ", "Ship   the board"},
			{"exactly 255 runes", strings.Repeat("a", 255), strings.Repeat("a", 255)},
			{"255 runes with padding", "  " + strings.Repeat("д", 255) + "  ", strings.Repeat("д", 255)},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, repo := tsNewSvc(t)
				var created *models.Task

				repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(1024.0, nil)
				tsExpectCreate(repo, &created)
				repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil)

				task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{Title: tc.title})

				require.NoError(t, err)
				require.NotNil(t, created)
				assert.Equal(t, tc.want, created.Title)
				assert.Equal(t, tc.want, task.Title)
			})
		}
	})

	t.Run("update rejects a blank title and leaves the row alone", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()
		current := &models.Task{ID: taskID, UserID: userID, Title: "Original", Status: models.StatusTodo}

		repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(current, nil)

		updated, err := svc.UpdateTask(t.Context(), userID, taskID, models.TaskPatch{Title: tsPtr("   ")})

		assert.Nil(t, updated)
		assert.ErrorIs(t, err, apperrors.ErrEmptyTaskTitle)
		repo.AssertNotCalled(t, "UpdateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

// ---------------------------------------------------------------------------
// status / priority validation
// ---------------------------------------------------------------------------

func TestTaskService_StatusAndPriorityValidation(t *testing.T) {
	userID := uuid.New()

	t.Run("create rejects unknown enum values", func(t *testing.T) {
		cases := []struct {
			name     string
			status   models.TaskStatus
			priority models.TaskPriority
			wantErr  error
		}{
			{"unknown status", models.TaskStatus("ARCHIVED"), models.PriorityHigh, apperrors.ErrInvalidTaskStatus},
			{"lower-case status", models.TaskStatus("todo"), models.PriorityHigh, apperrors.ErrInvalidTaskStatus},
			{"status with padding is not trimmed", models.TaskStatus(" TODO "), models.PriorityHigh, apperrors.ErrInvalidTaskStatus},
			{"unknown priority", models.StatusTodo, models.TaskPriority("BLOCKER"), apperrors.ErrInvalidTaskPriority},
			{"lower-case priority", models.StatusTodo, models.TaskPriority("high"), apperrors.ErrInvalidTaskPriority},
			{"status is checked before priority", models.TaskStatus("NOPE"), models.TaskPriority("NOPE"), apperrors.ErrInvalidTaskStatus},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, repo := tsNewSvc(t)

				task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{
					Title:    "Valid title",
					Status:   tc.status,
					Priority: tc.priority,
				})

				assert.Nil(t, task)
				assert.ErrorIs(t, err, tc.wantErr)
				repo.AssertNotCalled(t, "CreateTask", mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("create defaults empty enums to TODO and MEDIUM", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		var created *models.Task

		repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(2048.0, nil)
		tsExpectCreate(repo, &created)
		repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil)

		task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{Title: "Valid title"})

		require.NoError(t, err)
		assert.Equal(t, models.StatusTodo, task.Status)
		assert.Equal(t, models.PriorityMedium, task.Priority)
		assert.Equal(t, 2048.0, task.Position)
		assert.Nil(t, task.StartedAt)
		assert.Nil(t, task.CompletedAt)
	})

	t.Run("update rejects unknown enum values", func(t *testing.T) {
		cases := []struct {
			name    string
			patch   models.TaskPatch
			wantErr error
		}{
			{"unknown status", models.TaskPatch{Status: tsPtr(models.TaskStatus("SHIPPED"))}, apperrors.ErrInvalidTaskStatus},
			{"unknown priority", models.TaskPatch{Priority: tsPtr(models.TaskPriority("P0"))}, apperrors.ErrInvalidTaskPriority},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, repo := tsNewSvc(t)
				taskID := uuid.New()
				repo.On("GetTaskByID", mock.Anything, userID, taskID).
					Return(&models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo}, nil)

				updated, err := svc.UpdateTask(t.Context(), userID, taskID, tc.patch)

				assert.Nil(t, updated)
				assert.ErrorIs(t, err, tc.wantErr)
				repo.AssertNotCalled(t, "UpdateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("MoveTask rejects an invalid target column before touching the repo", func(t *testing.T) {
		svc, repo := tsNewSvc(t)

		moved, err := svc.MoveTask(t.Context(), userID, uuid.New(), models.TaskStatus("BACKLOG"), nil, nil)

		assert.Nil(t, moved)
		assert.ErrorIs(t, err, apperrors.ErrInvalidTaskStatus)
		repo.AssertNotCalled(t, "GetTaskByID", mock.Anything, mock.Anything, mock.Anything)
		repo.AssertNotCalled(t, "PositionBetween", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

// ---------------------------------------------------------------------------
// hours validation
// ---------------------------------------------------------------------------

func TestTaskService_HoursValidation(t *testing.T) {
	userID := uuid.New()
	const hoursErr = "hours must be between 0 and 1000"

	t.Run("out-of-range hours are rejected on create", func(t *testing.T) {
		cases := []struct {
			name string
			in   models.TaskCreate
		}{
			{"negative estimate", models.TaskCreate{Title: "T", EstimateHours: tsPtr(-0.5)}},
			{"very negative estimate", models.TaskCreate{Title: "T", EstimateHours: tsPtr(-1000.0)}},
			{"negative buffer", models.TaskCreate{Title: "T", BufferHours: tsPtr(-1.0)}},
			{"estimate over 1000", models.TaskCreate{Title: "T", EstimateHours: tsPtr(1000.01)}},
			{"buffer over 1000", models.TaskCreate{Title: "T", BufferHours: tsPtr(5000.0)}},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, repo := tsNewSvc(t)

				task, err := svc.CreateTask(t.Context(), userID, tc.in)

				assert.Nil(t, task)
				require.Error(t, err)
				assert.EqualError(t, err, hoursErr)
				repo.AssertNotCalled(t, "CreateTask", mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("in-range hours are accepted and rounded to 2 decimals", func(t *testing.T) {
		cases := []struct {
			name     string
			estimate float64
			want     float64
		}{
			{"zero is allowed", 0, 0},
			{"already 2 decimals", 2.5, 2.5},
			{"rounds up", 3.456, 3.46},
			{"rounds down", 12.344, 12.34},
			{"long tail", 1.0 / 3.0, 0.33},
			{"upper bound", 1000, 1000},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, repo := tsNewSvc(t)
				var created *models.Task

				repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(1024.0, nil)
				tsExpectCreate(repo, &created)
				repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil)

				task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{
					Title:         "Valid title",
					EstimateHours: tsPtr(tc.estimate),
				})

				require.NoError(t, err)
				require.NotNil(t, created.EstimateHours)
				assert.Equal(t, tc.want, *created.EstimateHours)
				assert.Equal(t, tc.want, *task.EstimateHours)
			})
		}
	})

	t.Run("update rounds every hours column it carries", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()
		current := models.Task{ID: taskID, UserID: userID, Status: models.StatusInProgress}
		var captured models.TaskPatch

		repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(&current, nil)
		tsExpectUpdate(repo, current, &captured)

		_, err := svc.UpdateTask(t.Context(), userID, taskID, models.TaskPatch{
			EstimateHours: tsPtr(tsPtr(3.456)),
			BufferHours:   tsPtr(tsPtr(12.344)),
			SpentHours:    tsPtr(tsPtr(7.129)),
		})

		require.NoError(t, err)
		assert.Equal(t, 3.46, **captured.EstimateHours)
		assert.Equal(t, 12.34, **captured.BufferHours)
		assert.Equal(t, 7.13, **captured.SpentHours)
	})

	t.Run("update rejects out-of-range spent hours", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()

		repo.On("GetTaskByID", mock.Anything, userID, taskID).
			Return(&models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo}, nil)

		updated, err := svc.UpdateTask(t.Context(), userID, taskID, models.TaskPatch{SpentHours: tsPtr(tsPtr(1001.0))})

		assert.Nil(t, updated)
		assert.EqualError(t, err, hoursErr)
		repo.AssertNotCalled(t, "UpdateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("a nil inner pointer clears the column without a range check", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()
		current := models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo, EstimateHours: tsPtr(4.0)}
		var captured models.TaskPatch

		repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(&current, nil)
		tsExpectUpdate(repo, current, &captured)

		updated, err := svc.UpdateTask(t.Context(), userID, taskID, models.TaskPatch{
			EstimateHours: tsPtr((*float64)(nil)),
		})

		require.NoError(t, err)
		require.NotNil(t, captured.EstimateHours)
		assert.Nil(t, *captured.EstimateHours)
		assert.Nil(t, updated.EstimateHours)
	})
}

// ---------------------------------------------------------------------------
// the IN_REVIEW rule
// ---------------------------------------------------------------------------

func TestTaskService_InReviewRequiresReviewer_OnCreate(t *testing.T) {
	userID := uuid.New()

	cases := []struct {
		name     string
		reviewer *string
		wantErr  error
		want     *string
	}{
		{"no reviewer at all", nil, apperrors.ErrReviewerRequired, nil},
		{"empty reviewer", tsPtr(""), apperrors.ErrReviewerRequired, nil},
		{"whitespace-only reviewer counts as absent", tsPtr("   \t\n "), apperrors.ErrReviewerRequired, nil},
		{"named reviewer", tsPtr("alice"), nil, tsPtr("alice")},
		{"reviewer is trimmed", tsPtr("  alice  "), nil, tsPtr("alice")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := tsNewSvc(t)
			var created *models.Task

			if tc.wantErr == nil {
				repo.On("AppendPosition", mock.Anything, userID, models.StatusInReview).Return(4096.0, nil)
				tsExpectCreate(repo, &created)
				repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil)
			}

			task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{
				Title:    "Needs a look",
				Status:   models.StatusInReview,
				Reviewer: tc.reviewer,
			})

			if tc.wantErr != nil {
				assert.Nil(t, task)
				assert.ErrorIs(t, err, tc.wantErr)
				repo.AssertNotCalled(t, "AppendPosition", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "CreateTask", mock.Anything, mock.Anything)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, task.Reviewer)
			assert.Equal(t, *tc.want, *task.Reviewer)
			assert.Equal(t, models.StatusInReview, task.Status)
			// IN_REVIEW is work in flight, so creating straight into it stamps started_at.
			assert.NotNil(t, task.StartedAt)
			assert.Nil(t, task.CompletedAt)
		})
	}
}

func TestTaskService_InReviewRequiresReviewer_OnUpdate(t *testing.T) {
	userID := uuid.New()

	cases := []struct {
		name string
		// state already persisted on the task
		currentStatus   models.TaskStatus
		currentReviewer *string
		// what the caller sends
		patchStatus   *models.TaskStatus
		patchReviewer **string
		wantErr       error
		// reviewer the repo ends up storing, once the patch is applied
		wantStoredReviewer *string
		// whether the patch touches the reviewer column at all
		wantReviewerInPatch bool
	}{
		{
			name:          "to IN_REVIEW with a reviewer in the patch",
			currentStatus: models.StatusTodo, currentReviewer: nil,
			patchStatus: tsPtr(models.StatusInReview), patchReviewer: tsPtr(tsPtr("carol")),
			wantStoredReviewer: tsPtr("carol"), wantReviewerInPatch: true,
		},
		{
			name:          "to IN_REVIEW with a reviewer already on the task",
			currentStatus: models.StatusInProgress, currentReviewer: tsPtr("bob"),
			patchStatus: tsPtr(models.StatusInReview), patchReviewer: nil,
			wantStoredReviewer: tsPtr("bob"), wantReviewerInPatch: false,
		},
		{
			name:          "to IN_REVIEW with neither",
			currentStatus: models.StatusTodo, currentReviewer: nil,
			patchStatus: tsPtr(models.StatusInReview), patchReviewer: nil,
			wantErr: apperrors.ErrReviewerRequired,
		},
		{
			name:          "to IN_REVIEW with a whitespace-only reviewer in the patch",
			currentStatus: models.StatusTodo, currentReviewer: nil,
			patchStatus: tsPtr(models.StatusInReview), patchReviewer: tsPtr(tsPtr("   ")),
			wantErr: apperrors.ErrReviewerRequired,
		},
		{
			name:          "to IN_REVIEW while clearing the reviewer the task had",
			currentStatus: models.StatusTodo, currentReviewer: tsPtr("bob"),
			patchStatus: tsPtr(models.StatusInReview), patchReviewer: tsPtr((*string)(nil)),
			wantErr: apperrors.ErrReviewerRequired,
		},
		{
			name:          "a whitespace-only reviewer in the patch overrides the one on the task",
			currentStatus: models.StatusTodo, currentReviewer: tsPtr("bob"),
			patchStatus: tsPtr(models.StatusInReview), patchReviewer: tsPtr(tsPtr(" \t ")),
			wantErr: apperrors.ErrReviewerRequired,
		},
		{
			name:          "a reviewer in the patch replaces the one on the task",
			currentStatus: models.StatusTodo, currentReviewer: tsPtr("bob"),
			patchStatus: tsPtr(models.StatusInReview), patchReviewer: tsPtr(tsPtr("  carol ")),
			wantStoredReviewer: tsPtr("carol"), wantReviewerInPatch: true,
		},
		{
			name:          "already IN_REVIEW, editing something else, reviewer still on the task",
			currentStatus: models.StatusInReview, currentReviewer: tsPtr("bob"),
			patchStatus: nil, patchReviewer: nil,
			wantStoredReviewer: tsPtr("bob"), wantReviewerInPatch: false,
		},
		{
			name:          "already IN_REVIEW, editing something else, no reviewer anywhere",
			currentStatus: models.StatusInReview, currentReviewer: nil,
			patchStatus: nil, patchReviewer: nil,
			wantErr: apperrors.ErrReviewerRequired,
		},
		{
			name:          "already IN_REVIEW, clearing the reviewer is refused",
			currentStatus: models.StatusInReview, currentReviewer: tsPtr("bob"),
			patchStatus: nil, patchReviewer: tsPtr((*string)(nil)),
			wantErr: apperrors.ErrReviewerRequired,
		},
		{
			name:          "leaving IN_REVIEW for DONE keeps the reviewer",
			currentStatus: models.StatusInReview, currentReviewer: tsPtr("bob"),
			patchStatus: tsPtr(models.StatusDone), patchReviewer: nil,
			wantStoredReviewer: tsPtr("bob"), wantReviewerInPatch: false,
		},
		{
			name:          "leaving IN_REVIEW for IN_PROGRESS keeps the reviewer",
			currentStatus: models.StatusInReview, currentReviewer: tsPtr("bob"),
			patchStatus: tsPtr(models.StatusInProgress), patchReviewer: nil,
			wantStoredReviewer: tsPtr("bob"), wantReviewerInPatch: false,
		},
		{
			name:          "leaving IN_REVIEW for TODO keeps the reviewer",
			currentStatus: models.StatusInReview, currentReviewer: tsPtr("bob"),
			patchStatus: tsPtr(models.StatusTodo), patchReviewer: nil,
			wantStoredReviewer: tsPtr("bob"), wantReviewerInPatch: false,
		},
		{
			name:          "a task outside IN_REVIEW may drop its reviewer",
			currentStatus: models.StatusDone, currentReviewer: tsPtr("bob"),
			patchStatus: nil, patchReviewer: tsPtr((*string)(nil)),
			wantStoredReviewer: nil, wantReviewerInPatch: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := tsNewSvc(t)
			taskID := uuid.New()
			current := models.Task{
				ID: taskID, UserID: userID, Title: "Needs a look",
				Status: tc.currentStatus, Reviewer: tc.currentReviewer,
				StartedAt: tsPtr(time.Now().UTC().Add(-time.Hour)),
			}
			var captured models.TaskPatch

			repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(&current, nil)
			if tc.wantErr == nil {
				tsExpectUpdate(repo, current, &captured)
				repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil).Maybe()
			}

			patch := models.TaskPatch{Status: tc.patchStatus, Reviewer: tc.patchReviewer}
			updated, err := svc.UpdateTask(t.Context(), userID, taskID, patch)

			if tc.wantErr != nil {
				assert.Nil(t, updated)
				assert.ErrorIs(t, err, tc.wantErr)
				repo.AssertNotCalled(t, "UpdateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				return
			}

			require.NoError(t, err)
			if tc.wantReviewerInPatch {
				require.NotNil(t, captured.Reviewer, "patch should carry the reviewer column")
			} else {
				assert.Nil(t, captured.Reviewer, "patch must not touch the reviewer column")
			}
			if tc.wantStoredReviewer == nil {
				assert.Nil(t, updated.Reviewer)
			} else {
				require.NotNil(t, updated.Reviewer)
				assert.Equal(t, *tc.wantStoredReviewer, *updated.Reviewer)
			}
		})
	}
}

func TestTaskService_UpdateTaskStatus_InReview(t *testing.T) {
	userID := uuid.New()

	t.Run("supplies the reviewer straight through", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()
		current := models.Task{ID: taskID, UserID: userID, Status: models.StatusInProgress}
		var captured models.TaskPatch

		repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(&current, nil)
		tsExpectUpdate(repo, current, &captured)
		repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil)

		err := svc.UpdateTaskStatus(t.Context(), taskID.String(), userID, models.StatusInReview, tsPtr("dana"))

		require.NoError(t, err)
		require.NotNil(t, captured.Reviewer)
		require.NotNil(t, *captured.Reviewer)
		assert.Equal(t, "dana", **captured.Reviewer)
	})

	t.Run("without a reviewer on an unreviewed task", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()

		repo.On("GetTaskByID", mock.Anything, userID, taskID).
			Return(&models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo}, nil)

		err := svc.UpdateTaskStatus(t.Context(), taskID.String(), userID, models.StatusInReview, nil)

		assert.ErrorIs(t, err, apperrors.ErrReviewerRequired)
	})

	t.Run("a malformed id is a not-found, not a panic", func(t *testing.T) {
		svc, repo := tsNewSvc(t)

		err := svc.UpdateTaskStatus(t.Context(), "not-a-uuid", userID, models.StatusDone, nil)

		assert.ErrorIs(t, err, apperrors.ErrTaskNotFound)
		repo.AssertNotCalled(t, "GetTaskByID", mock.Anything, mock.Anything, mock.Anything)
	})
}

// ---------------------------------------------------------------------------
// transition timestamps
// ---------------------------------------------------------------------------

func TestTaskService_TransitionTimestamps(t *testing.T) {
	userID := uuid.New()
	past := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	type stampWant int
	const (
		untouched stampWant = iota // patch leaves the column alone
		stamped                    // patch sets the column to now
		cleared                    // patch sets the column to NULL
	)

	cases := []struct {
		name            string
		currentStatus   models.TaskStatus
		currentStarted  *time.Time
		currentDone     *time.Time
		patchStatus     *models.TaskStatus
		patchTitle      *string
		wantStarted     stampWant
		wantCompleted   stampWant
		wantHistoryCall bool
	}{
		{
			name:          "TODO to IN_PROGRESS stamps started_at",
			currentStatus: models.StatusTodo, patchStatus: tsPtr(models.StatusInProgress),
			wantStarted: stamped, wantCompleted: untouched, wantHistoryCall: true,
		},
		{
			name:          "TODO to DONE stamps both",
			currentStatus: models.StatusTodo, patchStatus: tsPtr(models.StatusDone),
			wantStarted: stamped, wantCompleted: stamped, wantHistoryCall: true,
		},
		{
			name:          "IN_PROGRESS to DONE stamps completed_at only",
			currentStatus: models.StatusInProgress, currentStarted: &past, patchStatus: tsPtr(models.StatusDone),
			wantStarted: untouched, wantCompleted: stamped, wantHistoryCall: true,
		},
		{
			name:          "a second transition does not re-stamp an existing started_at",
			currentStatus: models.StatusInProgress, currentStarted: &past, patchStatus: tsPtr(models.StatusInReview),
			wantStarted: untouched, wantCompleted: untouched, wantHistoryCall: true,
		},
		{
			name:          "DONE to TODO clears completed_at",
			currentStatus: models.StatusDone, currentStarted: &past, currentDone: &past, patchStatus: tsPtr(models.StatusTodo),
			wantStarted: untouched, wantCompleted: cleared, wantHistoryCall: true,
		},
		{
			name:          "DONE to IN_PROGRESS clears completed_at and keeps started_at",
			currentStatus: models.StatusDone, currentStarted: &past, currentDone: &past, patchStatus: tsPtr(models.StatusInProgress),
			wantStarted: untouched, wantCompleted: cleared, wantHistoryCall: true,
		},
		{
			name:          "DONE to IN_PROGRESS on a row that never got a started_at backfills it",
			currentStatus: models.StatusDone, currentDone: &past, patchStatus: tsPtr(models.StatusInProgress),
			wantStarted: stamped, wantCompleted: cleared, wantHistoryCall: true,
		},
		{
			name:          "re-sending the same status stamps nothing",
			currentStatus: models.StatusInProgress, currentStarted: &past, patchStatus: tsPtr(models.StatusInProgress),
			wantStarted: untouched, wantCompleted: untouched, wantHistoryCall: false,
		},
		{
			name:          "re-sending DONE does not re-stamp completed_at",
			currentStatus: models.StatusDone, currentStarted: &past, currentDone: &past, patchStatus: tsPtr(models.StatusDone),
			wantStarted: untouched, wantCompleted: untouched, wantHistoryCall: false,
		},
		{
			name:          "a patch without a status stamps nothing",
			currentStatus: models.StatusInProgress, currentStarted: &past, patchTitle: tsPtr("Renamed"),
			wantStarted: untouched, wantCompleted: untouched, wantHistoryCall: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo := tsNewSvc(t)
			taskID := uuid.New()
			current := models.Task{
				ID: taskID, UserID: userID, Title: "Task", Status: tc.currentStatus,
				Reviewer: tsPtr("bob"), StartedAt: tc.currentStarted, CompletedAt: tc.currentDone,
			}
			var captured models.TaskPatch
			var change *models.StatusChange

			repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(&current, nil)
			tsExpectUpdate(repo, current, &captured)
			if tc.wantHistoryCall {
				repo.On("RecordStatusChange", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) { change = args.Get(1).(*models.StatusChange) }).
					Return(nil).Once()
			}

			before, after := tsWindow()
			updated, err := svc.UpdateTask(t.Context(), userID, taskID,
				models.TaskPatch{Status: tc.patchStatus, Title: tc.patchTitle})
			require.NoError(t, err)

			switch tc.wantStarted {
			case untouched:
				assert.Nil(t, captured.StartedAt, "started_at should not be in the patch")
				assert.Equal(t, tc.currentStarted, updated.StartedAt)
			case stamped:
				require.NotNil(t, captured.StartedAt)
				require.NotNil(t, *captured.StartedAt)
				assert.WithinRange(t, **captured.StartedAt, before, after())
			case cleared:
				require.NotNil(t, captured.StartedAt)
				assert.Nil(t, *captured.StartedAt)
			}

			switch tc.wantCompleted {
			case untouched:
				assert.Nil(t, captured.CompletedAt, "completed_at should not be in the patch")
				assert.Equal(t, tc.currentDone, updated.CompletedAt)
			case stamped:
				require.NotNil(t, captured.CompletedAt)
				require.NotNil(t, *captured.CompletedAt)
				assert.WithinRange(t, **captured.CompletedAt, before, after())
			case cleared:
				require.NotNil(t, captured.CompletedAt, "clearing means a non-nil outer pointer")
				assert.Nil(t, *captured.CompletedAt, "holding a nil time")
				assert.Nil(t, updated.CompletedAt)
			}

			if !tc.wantHistoryCall {
				require.Nil(t, change)
				repo.AssertNotCalled(t, "RecordStatusChange", mock.Anything, mock.Anything)
				return
			}

			require.NotNil(t, change)
			assert.Equal(t, taskID, change.TaskID)
			assert.Equal(t, userID, change.UserID)
			require.NotNil(t, change.FromStatus)
			assert.Equal(t, tc.currentStatus, *change.FromStatus)
			assert.Equal(t, *tc.patchStatus, change.ToStatus)
			assert.WithinRange(t, change.ChangedAt, before, after())
			// The history row and the timestamp columns share one clock reading.
			if tc.wantStarted == stamped {
				assert.Equal(t, change.ChangedAt, **captured.StartedAt)
			}
			if tc.wantCompleted == stamped {
				assert.Equal(t, change.ChangedAt, **captured.CompletedAt)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// status history
// ---------------------------------------------------------------------------

func TestTaskService_RecordStatusChange(t *testing.T) {
	userID := uuid.New()

	t.Run("create records one row with no from-status", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		var created *models.Task
		var change *models.StatusChange

		repo.On("AppendPosition", mock.Anything, userID, models.StatusInProgress).Return(1024.0, nil)
		tsExpectCreate(repo, &created)
		repo.On("RecordStatusChange", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { change = args.Get(1).(*models.StatusChange) }).
			Return(nil).Once()

		before, after := tsWindow()
		task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{
			Title:  "Start now",
			Status: models.StatusInProgress,
		})

		require.NoError(t, err)
		require.NotNil(t, change)
		assert.Nil(t, change.FromStatus, "a brand-new task has no previous status")
		assert.Equal(t, models.StatusInProgress, change.ToStatus)
		assert.Equal(t, task.ID, change.TaskID)
		assert.Equal(t, userID, change.UserID)
		assert.NotEqual(t, uuid.Nil, change.ID)
		assert.WithinRange(t, change.ChangedAt, before, after())
		require.NotNil(t, task.StartedAt)
		assert.Equal(t, change.ChangedAt, *task.StartedAt)
	})

	t.Run("history is an audit trail, so its failures are swallowed", func(t *testing.T) {
		t.Run("on create", func(t *testing.T) {
			svc, repo := tsNewSvc(t)
			var created *models.Task

			repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(1024.0, nil)
			tsExpectCreate(repo, &created)
			repo.On("RecordStatusChange", mock.Anything, mock.Anything).
				Return(errors.New("history table is on fire")).Once()

			task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{Title: "Still fine"})

			require.NoError(t, err)
			require.NotNil(t, task)
			assert.Equal(t, "Still fine", task.Title)
		})

		t.Run("on update", func(t *testing.T) {
			svc, repo := tsNewSvc(t)
			taskID := uuid.New()
			current := models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo}
			var captured models.TaskPatch

			repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(&current, nil)
			tsExpectUpdate(repo, current, &captured)
			repo.On("RecordStatusChange", mock.Anything, mock.Anything).
				Return(errors.New("history table is on fire")).Once()

			updated, err := svc.UpdateTask(t.Context(), userID, taskID,
				models.TaskPatch{Status: tsPtr(models.StatusInProgress)})

			require.NoError(t, err)
			require.NotNil(t, updated)
			assert.Equal(t, models.StatusInProgress, updated.Status)
		})
	})

	t.Run("one row per real transition across a chain of updates", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()
		stored := models.Task{ID: taskID, UserID: userID, Title: "Chain", Status: models.StatusTodo, Reviewer: tsPtr("bob")}
		// Two distinct rows: the service holds the read as `current` while the
		// write lands, so they must not alias each other.
		read, written := new(models.Task), new(models.Task)

		var recorded []models.StatusChange
		repo.On("GetTaskByID", mock.Anything, userID, taskID).
			Run(func(mock.Arguments) { *read = stored }).
			Return(read, nil)
		repo.On("UpdateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) {
				stored = tsApplyPatch(stored, args.Get(3).(models.TaskPatch))
				*written = stored
			}).
			Return(written, nil)
		repo.On("RecordStatusChange", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { recorded = append(recorded, *args.Get(1).(*models.StatusChange)) }).
			Return(nil)

		steps := []models.TaskStatus{
			models.StatusInProgress,
			models.StatusInProgress, // no-op, must not be recorded
			models.StatusInReview,
			models.StatusDone,
			models.StatusTodo,
		}
		for _, next := range steps {
			_, err := svc.UpdateTask(t.Context(), userID, taskID, models.TaskPatch{Status: tsPtr(next)})
			require.NoError(t, err)
		}

		require.Len(t, recorded, 4, "the repeated IN_PROGRESS must not produce a row")
		wantFrom := []models.TaskStatus{models.StatusTodo, models.StatusInProgress, models.StatusInReview, models.StatusDone}
		wantTo := []models.TaskStatus{models.StatusInProgress, models.StatusInReview, models.StatusDone, models.StatusTodo}
		for i, got := range recorded {
			require.NotNil(t, got.FromStatus, "step %d", i)
			assert.Equal(t, wantFrom[i], *got.FromStatus, "step %d", i)
			assert.Equal(t, wantTo[i], got.ToStatus, "step %d", i)
		}
	})
}

// ---------------------------------------------------------------------------
// MoveTask
// ---------------------------------------------------------------------------

func TestTaskService_MoveTask(t *testing.T) {
	userID := uuid.New()

	t.Run("asks the repo for a position and forwards it with the status", func(t *testing.T) {
		cases := []struct {
			name              string
			afterID, beforeID *uuid.UUID
			position          float64
		}{
			{"top of column", nil, tsPtr(uuid.New()), 512},
			{"between two cards", tsPtr(uuid.New()), tsPtr(uuid.New()), 1536},
			{"bottom of column", tsPtr(uuid.New()), nil, 3072},
			{"only card in column", nil, nil, 1024},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, repo := tsNewSvc(t)
				taskID := uuid.New()
				current := models.Task{
					ID: taskID, UserID: userID, Title: "Card",
					Status: models.StatusTodo, Position: 99,
				}
				var captured models.TaskPatch

				repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(&current, nil)
				repo.On("PositionBetween", mock.Anything, userID, models.StatusInProgress, tc.afterID, tc.beforeID).
					Return(tc.position, nil).Once()
				tsExpectUpdate(repo, current, &captured)
				repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil).Once()

				moved, err := svc.MoveTask(t.Context(), userID, taskID, models.StatusInProgress, tc.afterID, tc.beforeID)

				require.NoError(t, err)
				// One patch carries the whole move: column, order and the transition stamp.
				require.NotNil(t, captured.Status)
				assert.Equal(t, models.StatusInProgress, *captured.Status)
				require.NotNil(t, captured.Position)
				assert.Equal(t, tc.position, *captured.Position)
				require.NotNil(t, captured.StartedAt)
				assert.NotNil(t, *captured.StartedAt)
				assert.Equal(t, tc.position, moved.Position)
				assert.Equal(t, models.StatusInProgress, moved.Status)
			})
		}
	})

	t.Run("reordering inside the same column moves without a history row", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()
		current := models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo, Position: 1024}
		var captured models.TaskPatch

		repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(&current, nil)
		repo.On("PositionBetween", mock.Anything, userID, models.StatusTodo, mock.Anything, mock.Anything).
			Return(2048.0, nil).Once()
		tsExpectUpdate(repo, current, &captured)

		moved, err := svc.MoveTask(t.Context(), userID, taskID, models.StatusTodo, nil, nil)

		require.NoError(t, err)
		assert.Equal(t, 2048.0, *captured.Position)
		assert.Nil(t, captured.StartedAt)
		assert.Nil(t, captured.CompletedAt)
		assert.Equal(t, 2048.0, moved.Position)
		repo.AssertNotCalled(t, "RecordStatusChange", mock.Anything, mock.Anything)
	})

	t.Run("moving into IN_REVIEW still needs a reviewer", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()

		repo.On("GetTaskByID", mock.Anything, userID, taskID).
			Return(&models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo}, nil)
		repo.On("PositionBetween", mock.Anything, userID, models.StatusInReview, mock.Anything, mock.Anything).
			Return(1024.0, nil)

		moved, err := svc.MoveTask(t.Context(), userID, taskID, models.StatusInReview, nil, nil)

		assert.Nil(t, moved)
		assert.ErrorIs(t, err, apperrors.ErrReviewerRequired)
		repo.AssertNotCalled(t, "UpdateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("moving into IN_REVIEW succeeds when the task already has a reviewer", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()
		current := models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo, Reviewer: tsPtr("bob")}
		var captured models.TaskPatch

		repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(&current, nil)
		repo.On("PositionBetween", mock.Anything, userID, models.StatusInReview, mock.Anything, mock.Anything).
			Return(4096.0, nil)
		tsExpectUpdate(repo, current, &captured)
		repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil).Once()

		moved, err := svc.MoveTask(t.Context(), userID, taskID, models.StatusInReview, nil, nil)

		require.NoError(t, err)
		assert.Equal(t, models.StatusInReview, moved.Status)
		assert.Equal(t, 4096.0, *captured.Position)
		assert.Nil(t, captured.Reviewer)
	})

	t.Run("a position failure aborts the move", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		taskID := uuid.New()

		repo.On("GetTaskByID", mock.Anything, userID, taskID).
			Return(&models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo}, nil)
		repo.On("PositionBetween", mock.Anything, userID, models.StatusDone, mock.Anything, mock.Anything).
			Return(0.0, errors.New("deadlock detected"))

		moved, err := svc.MoveTask(t.Context(), userID, taskID, models.StatusDone, nil, nil)

		assert.Nil(t, moved)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "position between")
		repo.AssertNotCalled(t, "UpdateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

// ---------------------------------------------------------------------------
// CreateTasksFromPlan
// ---------------------------------------------------------------------------

func tsPlanWeek(index int, endsOn string, tasks ...models.PlannedTask) models.PlannedWeek {
	return models.PlannedWeek{Index: index, EndsOn: endsOn, Tasks: tasks}
}

func TestTaskService_CreateTasksFromPlan(t *testing.T) {
	userID := uuid.New()

	t.Run("an empty plan is rejected before any write", func(t *testing.T) {
		cases := []struct {
			name string
			plan *models.SprintPlan
		}{
			{"nil plan", nil},
			{"nil weeks", &models.SprintPlan{Name: "Sprint 1"}},
			{"empty weeks", &models.SprintPlan{Name: "Sprint 1", Weeks: []models.PlannedWeek{}}},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, repo := tsNewSvc(t)

				tasks, err := svc.CreateTasksFromPlan(t.Context(), userID, nil, tc.plan)

				assert.Nil(t, tasks)
				assert.ErrorIs(t, err, apperrors.ErrNoPlanningItems)
				repo.AssertNotCalled(t, "AppendPosition", mock.Anything, mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "CreateTasksBulk", mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("weeks whose items are all untitled yield nothing to create", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(1024.0, nil)

		tasks, err := svc.CreateTasksFromPlan(t.Context(), userID, nil, &models.SprintPlan{
			Weeks: []models.PlannedWeek{
				tsPlanWeek(1, "2026-08-07",
					models.PlannedTask{Title: "   "},
					models.PlannedTask{Title: ""},
				),
			},
		})

		assert.Nil(t, tasks)
		assert.ErrorIs(t, err, apperrors.ErrNoPlanningItems)
		repo.AssertNotCalled(t, "CreateTasksBulk", mock.Anything, mock.Anything)
	})

	t.Run("subtasks become a markdown checklist on the description", func(t *testing.T) {
		cases := []struct {
			name        string
			description string
			subtasks    []models.PlannedSubtask
			want        *string
		}{
			{
				name:        "description plus checklist",
				description: "Wire the board to the API.",
				subtasks: []models.PlannedSubtask{
					{Title: "Write the endpoint", EstimateHours: 1.5},
					{Title: "Write the tests", EstimateHours: 2},
				},
				want: tsPtr("Wire the board to the API.\n\n- [ ] Write the endpoint (1.5h)\n- [ ] Write the tests (2h)"),
			},
			{
				name:        "checklist only",
				description: "   ",
				subtasks:    []models.PlannedSubtask{{Title: "  Draft the schema  ", EstimateHours: 0.25}},
				want:        tsPtr("- [ ] Draft the schema (0.25h)"),
			},
			{
				name:        "blank subtask titles are skipped",
				description: "Notes",
				subtasks: []models.PlannedSubtask{
					{Title: "  ", EstimateHours: 1},
					{Title: "Real one", EstimateHours: 3},
				},
				want: tsPtr("Notes\n\n- [ ] Real one (3h)"),
			},
			{
				name:        "no subtasks leaves the description alone",
				description: "  Just notes  ",
				subtasks:    nil,
				want:        tsPtr("Just notes"),
			},
			{
				name:        "nothing at all collapses to NULL",
				description: "",
				subtasks:    nil,
				want:        nil,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				svc, repo := tsNewSvc(t)
				var bulk []models.Task

				repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(1024.0, nil)
				repo.On("CreateTasksBulk", mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) { bulk = args.Get(1).([]models.Task) }).
					Return(nil)
				repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil)

				tasks, err := svc.CreateTasksFromPlan(t.Context(), userID, nil, &models.SprintPlan{
					Weeks: []models.PlannedWeek{tsPlanWeek(1, "2026-08-07", models.PlannedTask{
						Title:       "Build the board",
						Description: tc.description,
						Subtasks:    tc.subtasks,
					})},
				})

				require.NoError(t, err)
				require.Len(t, tasks, 1)
				require.Len(t, bulk, 1)
				if tc.want == nil {
					assert.Nil(t, bulk[0].Description)
					return
				}
				require.NotNil(t, bulk[0].Description)
				assert.Equal(t, *tc.want, *bulk[0].Description)
				assert.Equal(t, *tc.want, *tasks[0].Description)
			})
		}
	})

	t.Run("a task needing review still lands in TODO", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		var bulk []models.Task

		repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(1024.0, nil)
		repo.On("CreateTasksBulk", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { bulk = args.Get(1).([]models.Task) }).
			Return(nil)
		repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil)

		tasks, err := svc.CreateTasksFromPlan(t.Context(), userID, nil, &models.SprintPlan{
			Weeks: []models.PlannedWeek{tsPlanWeek(1, "2026-08-07",
				models.PlannedTask{Title: "Reviewed work", NeedsReview: true, Reviewer: "  bob  "},
				models.PlannedTask{Title: "Review flagged, nobody named", NeedsReview: true, Order: 1},
			)},
		})

		require.NoError(t, err)
		require.Len(t, bulk, 2)

		// A reviewer on a plan item is stored, but planning never opens a task
		// straight into IN_REVIEW.
		assert.Equal(t, models.StatusTodo, bulk[0].Status)
		require.NotNil(t, bulk[0].Reviewer)
		assert.Equal(t, "bob", *bulk[0].Reviewer)

		assert.Equal(t, models.StatusTodo, bulk[1].Status)
		assert.Nil(t, bulk[1].Reviewer)

		for _, task := range tasks {
			assert.Equal(t, models.StatusTodo, task.Status)
			assert.Nil(t, task.StartedAt)
			assert.Nil(t, task.CompletedAt)
		}
	})

	t.Run("positions increment so board order matches plan order", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		sprintID := uuid.New()
		var bulk []models.Task
		var recorded []models.StatusChange

		repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(5120.0, nil).Once()
		repo.On("CreateTasksBulk", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { bulk = args.Get(1).([]models.Task) }).
			Return(nil).Once()
		repo.On("RecordStatusChange", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { recorded = append(recorded, *args.Get(1).(*models.StatusChange)) }).
			Return(nil)

		// Weeks and tasks arrive shuffled; the service must sort by Index then Order.
		plan := &models.SprintPlan{Weeks: []models.PlannedWeek{
			tsPlanWeek(2, "2026-08-14",
				models.PlannedTask{Title: "Third", Order: 0},
				models.PlannedTask{Title: "Fourth", Order: 1},
			),
			tsPlanWeek(1, "2026-08-07",
				models.PlannedTask{Title: "Second", Order: 5},
				models.PlannedTask{Title: "First", Order: 2},
			),
		}}

		tasks, err := svc.CreateTasksFromPlan(t.Context(), userID, &sprintID, plan)

		require.NoError(t, err)
		require.Len(t, tasks, 4)
		assert.Equal(t, bulk, tasks, "the returned slice is what was written")

		wantTitles := []string{"First", "Second", "Third", "Fourth"}
		wantPositions := []float64{5120, 6144, 7168, 8192}
		wantDue := []time.Time{
			time.Date(2026, 8, 7, 23, 59, 59, 0, time.UTC),
			time.Date(2026, 8, 7, 23, 59, 59, 0, time.UTC),
			time.Date(2026, 8, 14, 23, 59, 59, 0, time.UTC),
			time.Date(2026, 8, 14, 23, 59, 59, 0, time.UTC),
		}
		for i, task := range tasks {
			assert.Equal(t, wantTitles[i], task.Title, "slot %d", i)
			assert.Equal(t, wantPositions[i], task.Position, "slot %d", i)
			assert.Equal(t, models.StatusTodo, task.Status, "slot %d", i)
			assert.Equal(t, models.PriorityMedium, task.Priority, "unset priority defaults, slot %d", i)
			require.NotNil(t, task.SprintID, "slot %d", i)
			assert.Equal(t, sprintID, *task.SprintID, "slot %d", i)
			require.NotNil(t, task.DueDate, "slot %d", i)
			assert.Equal(t, wantDue[i], *task.DueDate, "slot %d", i)
			assert.NotEqual(t, uuid.Nil, task.ID, "slot %d", i)
		}

		require.Len(t, recorded, 4, "one history row per planned task")
		for i, change := range recorded {
			assert.Nil(t, change.FromStatus, "row %d", i)
			assert.Equal(t, models.StatusTodo, change.ToStatus, "row %d", i)
			assert.Equal(t, tasks[i].ID, change.TaskID, "row %d", i)
		}
	})

	t.Run("plan hours and priority are normalised", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		var bulk []models.Task

		repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(1024.0, nil)
		repo.On("CreateTasksBulk", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { bulk = args.Get(1).([]models.Task) }).
			Return(nil)
		repo.On("RecordStatusChange", mock.Anything, mock.Anything).Return(nil)

		_, err := svc.CreateTasksFromPlan(t.Context(), userID, nil, &models.SprintPlan{
			Weeks: []models.PlannedWeek{tsPlanWeek(1, "not-a-date",
				models.PlannedTask{Title: "Rounded", EstimateHours: 3.456, BufferHours: 12.344, Priority: models.PriorityUrgent},
				models.PlannedTask{Title: "Zeroed", Order: 1, EstimateHours: 0, BufferHours: -4},
				models.PlannedTask{Title: "Clamped", Order: 2, EstimateHours: 9999, Priority: models.TaskPriority("EPIC")},
			)},
		})

		require.NoError(t, err)
		require.Len(t, bulk, 3)

		assert.Equal(t, 3.46, *bulk[0].EstimateHours)
		assert.Equal(t, 12.34, *bulk[0].BufferHours)
		assert.Equal(t, models.PriorityUrgent, bulk[0].Priority)

		assert.Nil(t, bulk[1].EstimateHours, "zero hours means unestimated, not 0")
		assert.Nil(t, bulk[1].BufferHours)

		assert.Equal(t, 1000.0, *bulk[2].EstimateHours, "clamped to the column maximum")
		assert.Equal(t, models.PriorityMedium, bulk[2].Priority, "an unknown priority falls back")

		for i, task := range bulk {
			assert.Nil(t, task.DueDate, "an unparsable end date leaves due_date NULL, slot %d", i)
		}
	})

	t.Run("a bulk write failure is surfaced and nothing is recorded", func(t *testing.T) {
		svc, repo := tsNewSvc(t)

		repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(1024.0, nil)
		repo.On("CreateTasksBulk", mock.Anything, mock.Anything).Return(errors.New("constraint violation"))

		tasks, err := svc.CreateTasksFromPlan(t.Context(), userID, nil, &models.SprintPlan{
			Weeks: []models.PlannedWeek{tsPlanWeek(1, "2026-08-07", models.PlannedTask{Title: "Doomed"})},
		})

		assert.Nil(t, tasks)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create tasks bulk")
		repo.AssertNotCalled(t, "RecordStatusChange", mock.Anything, mock.Anything)
	})
}

// ---------------------------------------------------------------------------
// repository error propagation
// ---------------------------------------------------------------------------

func TestTaskService_RepositoryErrors(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	t.Run("not-found passes through untouched", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(nil, apperrors.ErrTaskNotFound)

		task, err := svc.GetTask(t.Context(), userID, taskID)

		assert.Nil(t, task)
		assert.ErrorIs(t, err, apperrors.ErrTaskNotFound)
	})

	t.Run("an unexpected read error is wrapped", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		repo.On("GetTaskByID", mock.Anything, userID, taskID).Return(nil, errors.New("connection refused"))

		task, err := svc.GetTask(t.Context(), userID, taskID)

		assert.Nil(t, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get task")
	})

	t.Run("nothing-to-update passes through untouched", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		repo.On("GetTaskByID", mock.Anything, userID, taskID).
			Return(&models.Task{ID: taskID, UserID: userID, Status: models.StatusTodo}, nil)
		repo.On("UpdateTask", mock.Anything, userID, taskID, mock.Anything).
			Return(nil, apperrors.ErrNothingToUpdate)

		updated, err := svc.UpdateTask(t.Context(), userID, taskID, models.TaskPatch{})

		assert.Nil(t, updated)
		assert.ErrorIs(t, err, apperrors.ErrNothingToUpdate)
		repo.AssertNotCalled(t, "RecordStatusChange", mock.Anything, mock.Anything)
	})

	t.Run("a failed create is wrapped and never recorded", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(1024.0, nil)
		repo.On("CreateTask", mock.Anything, mock.Anything).Return(errors.New("disk full"))

		task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{Title: "Doomed"})

		assert.Nil(t, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "create task")
		repo.AssertNotCalled(t, "RecordStatusChange", mock.Anything, mock.Anything)
	})

	t.Run("a failed append position aborts the create", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		repo.On("AppendPosition", mock.Anything, userID, models.StatusTodo).Return(0.0, errors.New("timeout"))

		task, err := svc.CreateTask(t.Context(), userID, models.TaskCreate{Title: "Doomed"})

		assert.Nil(t, task)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "append position")
		repo.AssertNotCalled(t, "CreateTask", mock.Anything, mock.Anything)
	})

	t.Run("list and delete forward the repo verdict", func(t *testing.T) {
		svc, repo := tsNewSvc(t)
		filter := models.TaskFilter{Statuses: []models.TaskStatus{models.StatusTodo}, Search: "board"}
		want := []models.Task{{ID: taskID, Title: "Board"}}

		repo.On("ListTasks", mock.Anything, userID, filter).Return(want, nil)
		repo.On("DeleteTask", mock.Anything, userID, taskID).Return(apperrors.ErrTaskNotFound)

		got, err := svc.GetTasks(t.Context(), userID, filter)
		require.NoError(t, err)
		assert.Equal(t, want, got)

		assert.ErrorIs(t, svc.DeleteTask(t.Context(), userID, taskID), apperrors.ErrTaskNotFound)
	})
}
