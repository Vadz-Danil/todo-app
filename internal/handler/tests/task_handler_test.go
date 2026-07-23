package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/handler"
	"todo-app/internal/middleware"
	"todo-app/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

// MockTaskService implements service.Task.
type MockTaskService struct {
	mock.Mock
}

func (m *MockTaskService) CreateTask(ctx context.Context, userID uuid.UUID, in models.TaskCreate) (*models.Task, error) {
	args := m.Called(ctx, userID, in)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Task), args.Error(1)
}

func (m *MockTaskService) GetTasks(ctx context.Context, userID uuid.UUID, filter models.TaskFilter) ([]models.Task, error) {
	args := m.Called(ctx, userID, filter)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Task), args.Error(1)
}

func (m *MockTaskService) GetTask(ctx context.Context, userID, taskID uuid.UUID) (*models.Task, error) {
	args := m.Called(ctx, userID, taskID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Task), args.Error(1)
}

func (m *MockTaskService) UpdateTask(ctx context.Context, userID, taskID uuid.UUID, patch models.TaskPatch) (*models.Task, error) {
	args := m.Called(ctx, userID, taskID, patch)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Task), args.Error(1)
}

func (m *MockTaskService) UpdateTaskStatus(ctx context.Context, taskID string, userID uuid.UUID, status models.TaskStatus, reviewer *string) error {
	args := m.Called(ctx, taskID, userID, status, reviewer)
	return args.Error(0)
}

func (m *MockTaskService) MoveTask(ctx context.Context, userID, taskID uuid.UUID, status models.TaskStatus, afterID, beforeID *uuid.UUID) (*models.Task, error) {
	args := m.Called(ctx, userID, taskID, status, afterID, beforeID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Task), args.Error(1)
}

func (m *MockTaskService) DeleteTask(ctx context.Context, userID, taskID uuid.UUID) error {
	args := m.Called(ctx, userID, taskID)
	return args.Error(0)
}

func (m *MockTaskService) CreateTasksFromPlan(ctx context.Context, userID uuid.UUID, sprintID *uuid.UUID, plan *models.SprintPlan) ([]models.Task, error) {
	args := m.Called(ctx, userID, sprintID, plan)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Task), args.Error(1)
}

type MockEmailService struct {
	mock.Mock
}

func (m *MockEmailService) ShareTasks(ctx context.Context, recipientEmail, senderEmail, dashboardURL string) error {
	args := m.Called(ctx, recipientEmail, senderEmail, dashboardURL)
	return args.Error(0)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func setupTaskRouter(taskService *MockTaskService, emailService *MockEmailService, authService *MockAuthService, userID uuid.UUID) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	if userID != uuid.Nil {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.UserIDKey, userID)
			c.Next()
		})
	}
	frontendURL := "http://localhost:3000"
	h := handler.NewTaskHandler(taskService, emailService, authService, frontendURL)

	r.POST("/tasks", h.CreateTask)
	r.GET("/tasks", h.GetTasks)
	r.POST("/tasks/share", h.ShareTasks)
	r.GET("/tasks/:id", h.GetTask)
	r.PATCH("/tasks/:id", h.UpdateTask)
	r.DELETE("/tasks/:id", h.DeleteTask)
	r.PATCH("/tasks/:id/status", h.UpdateTaskStatus)
	r.PATCH("/tasks/:id/move", h.MoveTask)

	return r
}

// doRequest fires a JSON request at the router. A nil body sends no payload,
// which lets us exercise the "no payload at all" branches.
func doRequest(r *gin.Engine, method, path string, body []byte) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	return w
}

func decodeTask(t *testing.T, raw []byte) models.Task {
	t.Helper()
	var task models.Task
	require.NoError(t, json.Unmarshal(raw, &task))
	return task
}

// sampleTask is a fully-populated task used as a canned service response.
func sampleTask(userID uuid.UUID, id uuid.UUID) *models.Task {
	return &models.Task{
		ID:       id,
		UserID:   userID,
		Title:    "Ship the board",
		Status:   models.StatusInProgress,
		Priority: models.PriorityHigh,
		Position: 2048,
	}
}

// ---------------------------------------------------------------------------
// POST /tasks
// ---------------------------------------------------------------------------

func TestTaskHandler_CreateTask(t *testing.T) {
	userID := uuid.New()
	sprintID := uuid.New()

	t.Run("Success - 201 Created", func(t *testing.T) {
		mockTask := new(MockTaskService)
		expectedTask := &models.Task{
			ID:          uuid.New(),
			UserID:      userID,
			Title:       "New Task",
			Description: new("Task Description"),
		}

		var got models.TaskCreate
		mockTask.On("CreateTask", mock.Anything, userID, mock.Anything).
			Run(func(args mock.Arguments) { got = args.Get(2).(models.TaskCreate) }).
			Return(expectedTask, nil)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		body, _ := json.Marshal(map[string]string{
			"title":       "New Task",
			"description": "Task Description",
		})

		w := doRequest(r, http.MethodPost, "/tasks", body)

		assert.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, w.Body.String(), "New Task")
		assert.Equal(t, "New Task", got.Title)
		require.NotNil(t, got.Description)
		assert.Equal(t, "Task Description", *got.Description)
		mockTask.AssertExpectations(t)
	})

	t.Run("Forwards the full models.TaskCreate", func(t *testing.T) {
		mockTask := new(MockTaskService)

		var got models.TaskCreate
		mockTask.On("CreateTask", mock.Anything, userID, mock.Anything).
			Run(func(args mock.Arguments) { got = args.Get(2).(models.TaskCreate) }).
			Return(&models.Task{ID: uuid.New(), UserID: userID, Title: "Review PR"}, nil)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		body := []byte(`{
			"title": "Review PR",
			"description": "check the migrations",
			"status": "IN_REVIEW",
			"priority": "URGENT",
			"reviewer": "alice",
			"estimate_hours": 3.5,
			"buffer_hours": 1,
			"blockers": "waiting on staging",
			"sprint_id": "` + sprintID.String() + `",
			"due_date": "2026-08-01"
		}`)

		w := doRequest(r, http.MethodPost, "/tasks", body)

		require.Equal(t, http.StatusCreated, w.Code)
		assert.Equal(t, "Review PR", got.Title)
		assert.Equal(t, models.StatusInReview, got.Status)
		assert.Equal(t, models.PriorityUrgent, got.Priority)
		require.NotNil(t, got.Reviewer)
		assert.Equal(t, "alice", *got.Reviewer)
		require.NotNil(t, got.EstimateHours)
		assert.InDelta(t, 3.5, *got.EstimateHours, 0.0001)
		require.NotNil(t, got.BufferHours)
		assert.InDelta(t, 1, *got.BufferHours, 0.0001)
		require.NotNil(t, got.Blockers)
		assert.Equal(t, "waiting on staging", *got.Blockers)
		require.NotNil(t, got.SprintID)
		assert.Equal(t, sprintID, *got.SprintID)
		require.NotNil(t, got.DueDate)
		assert.Equal(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), got.DueDate.UTC())
		mockTask.AssertExpectations(t)
	})

	t.Run("Bad payloads - 400 Bad Request", func(t *testing.T) {
		tests := []struct {
			name string
			body []byte
		}{
			{name: "malformed JSON", body: []byte(`{"title": "New Task"`)},
			{name: "not an object", body: []byte(`"just a string"`)},
			{name: "missing title", body: []byte(`{"description":"orphan"}`)},
			{name: "empty title", body: []byte(`{"title":""}`)},
			{name: "wrong field type", body: []byte(`{"title": 42}`)},
			{name: "invalid sprint_id", body: []byte(`{"title":"x","sprint_id":"not-a-uuid"}`)},
			{name: "invalid due_date", body: []byte(`{"title":"x","due_date":"01/08/2026"}`)},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				mockTask := new(MockTaskService)
				r := setupTaskRouter(mockTask, nil, nil, userID)

				w := doRequest(r, http.MethodPost, "/tasks", tc.body)

				assert.Equal(t, http.StatusBadRequest, w.Code)
				mockTask.AssertNotCalled(t, "CreateTask", mock.Anything, mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("Service failure - 500 Internal Server Error", func(t *testing.T) {
		mockTask := new(MockTaskService)
		mockTask.On("CreateTask", mock.Anything, userID, mock.Anything).
			Return(nil, assert.AnError)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		w := doRequest(r, http.MethodPost, "/tasks", []byte(`{"title":"boom"}`))

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to create task")
		mockTask.AssertExpectations(t)
	})

	t.Run("Unauthorized - 401 Missing Context UserID", func(t *testing.T) {
		mockTask := new(MockTaskService)
		r := setupTaskRouter(mockTask, nil, nil, uuid.Nil)

		w := doRequest(r, http.MethodPost, "/tasks", nil)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ---------------------------------------------------------------------------
// GET /tasks
// ---------------------------------------------------------------------------

func TestTaskHandler_GetTasks_Filter(t *testing.T) {
	userID := uuid.New()
	sprintID := uuid.New()

	tests := []struct {
		name       string
		query      string
		wantFilter models.TaskFilter
	}{
		{
			name:  "statuses, priority and search",
			query: "?status=TODO,IN_REVIEW&priority=URGENT&q=foo",
			wantFilter: models.TaskFilter{
				Statuses:   []models.TaskStatus{models.StatusTodo, models.StatusInReview},
				Priorities: []models.TaskPriority{models.PriorityUrgent},
				Search:     "foo",
			},
		},
		{
			name:       "no query means no filter",
			query:      "",
			wantFilter: models.TaskFilter{},
		},
		{
			name:  "unknown enum values are ignored, not rejected",
			query: "?status=TODO,NOPE,,IN_REVIEW&priority=BOGUS",
			wantFilter: models.TaskFilter{
				Statuses: []models.TaskStatus{models.StatusTodo, models.StatusInReview},
			},
		},
		{
			name:       "every enum value unknown leaves the filter empty",
			query:      "?status=ARCHIVED&priority=CRITICAL",
			wantFilter: models.TaskFilter{},
		},
		{
			name:  "values are upper-cased and trimmed",
			query: "?status=todo,%20in_progress%20&priority=%20low%20&q=%20spaced%20",
			wantFilter: models.TaskFilter{
				Statuses:   []models.TaskStatus{models.StatusTodo, models.StatusInProgress},
				Priorities: []models.TaskPriority{models.PriorityLow},
				Search:     "spaced",
			},
		},
		{
			name:       "sprint_id is parsed into a pointer",
			query:      "?sprint_id=" + sprintID.String(),
			wantFilter: models.TaskFilter{SprintID: &sprintID},
		},
		{
			name:  "from and to are parsed",
			query: "?from=2026-07-01&to=2026-07-31T23:59:59Z",
			wantFilter: models.TaskFilter{
				From: new(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
				To:   new(time.Date(2026, 7, 31, 23, 59, 59, 0, time.UTC)),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockTask := new(MockTaskService)

			var got models.TaskFilter
			mockTask.On("GetTasks", mock.Anything, userID, mock.Anything).
				Run(func(args mock.Arguments) { got = args.Get(2).(models.TaskFilter) }).
				Return([]models.Task{}, nil)

			r := setupTaskRouter(mockTask, nil, nil, userID)

			w := doRequest(r, http.MethodGet, "/tasks"+tc.query, nil)

			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, tc.wantFilter, got)
			mockTask.AssertExpectations(t)
		})
	}
}

func TestTaskHandler_GetTasks(t *testing.T) {
	userID := uuid.New()

	t.Run("Success - 200 OK", func(t *testing.T) {
		mockTask := new(MockTaskService)
		tasks := []models.Task{*sampleTask(userID, uuid.New())}

		mockTask.On("GetTasks", mock.Anything, userID, mock.Anything).Return(tasks, nil)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		w := doRequest(r, http.MethodGet, "/tasks", nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Ship the board")
		mockTask.AssertExpectations(t)
	})

	t.Run("Nil slice is serialised as an empty array", func(t *testing.T) {
		mockTask := new(MockTaskService)
		mockTask.On("GetTasks", mock.Anything, userID, mock.Anything).Return(nil, nil)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		w := doRequest(r, http.MethodGet, "/tasks", nil)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.JSONEq(t, `{"tasks":[]}`, w.Body.String())
		mockTask.AssertExpectations(t)
	})

	t.Run("Invalid sprint_id - 400 Bad Request", func(t *testing.T) {
		mockTask := new(MockTaskService)
		r := setupTaskRouter(mockTask, nil, nil, userID)

		w := doRequest(r, http.MethodGet, "/tasks?sprint_id=nope", nil)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid sprint_id format")
		mockTask.AssertNotCalled(t, "GetTasks", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("Invalid from - 400 Bad Request", func(t *testing.T) {
		mockTask := new(MockTaskService)
		r := setupTaskRouter(mockTask, nil, nil, userID)

		w := doRequest(r, http.MethodGet, "/tasks?from=yesterday", nil)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		mockTask.AssertNotCalled(t, "GetTasks", mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("Unauthorized - 401 Missing Context UserID", func(t *testing.T) {
		mockTask := new(MockTaskService)
		r := setupTaskRouter(mockTask, nil, nil, uuid.Nil)

		w := doRequest(r, http.MethodGet, "/tasks", nil)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

// ---------------------------------------------------------------------------
// GET /tasks/:id
// ---------------------------------------------------------------------------

func TestTaskHandler_GetTask(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	t.Run("Success - 200 OK", func(t *testing.T) {
		mockTask := new(MockTaskService)
		mockTask.On("GetTask", mock.Anything, userID, taskID).Return(sampleTask(userID, taskID), nil)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		w := doRequest(r, http.MethodGet, "/tasks/"+taskID.String(), nil)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, taskID, decodeTask(t, w.Body.Bytes()).ID)
		mockTask.AssertExpectations(t)
	})

	t.Run("Not Found - 404 Not Found", func(t *testing.T) {
		mockTask := new(MockTaskService)
		mockTask.On("GetTask", mock.Anything, userID, taskID).Return(nil, apperrors.ErrTaskNotFound)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		w := doRequest(r, http.MethodGet, "/tasks/"+taskID.String(), nil)

		assert.Equal(t, http.StatusNotFound, w.Code)
		mockTask.AssertExpectations(t)
	})

	t.Run("Invalid Task UUID Format - 400 Bad Request", func(t *testing.T) {
		mockTask := new(MockTaskService)
		r := setupTaskRouter(mockTask, nil, nil, userID)

		w := doRequest(r, http.MethodGet, "/tasks/invalid-uuid-format", nil)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid id format")
		mockTask.AssertNotCalled(t, "GetTask", mock.Anything, mock.Anything, mock.Anything)
	})
}

// ---------------------------------------------------------------------------
// PATCH /tasks/:id — absent vs. null
// ---------------------------------------------------------------------------

// TestTaskHandler_UpdateTask_PatchSemantics pins the subtlest rule of the
// request layer: an absent field must reach the service as a nil outer pointer
// ("leave alone"), while an explicit null must reach it as a non-nil outer
// pointer holding a nil inner pointer ("clear this column").
func TestTaskHandler_UpdateTask_PatchSemantics(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()
	sprintID := uuid.New()

	tests := []struct {
		name   string
		body   string
		assert func(t *testing.T, patch models.TaskPatch)
	}{
		{
			name: "explicit null clears description",
			body: `{"description": null}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.Description, "outer pointer must be set: the field was present")
				assert.Nil(t, *patch.Description, "inner pointer must be nil: null means clear")
			},
		},
		{
			name: "absent field leaves description untouched",
			body: `{}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				assert.Nil(t, patch.Description, "absent field must not produce a patch entry")
			},
		},
		{
			name: "another field present still leaves description untouched",
			body: `{"title": "renamed"}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				assert.Nil(t, patch.Description)
				require.NotNil(t, patch.Title)
				assert.Equal(t, "renamed", *patch.Title)
			},
		},
		{
			name: "value sets description",
			body: `{"description": "now with detail"}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.Description)
				require.NotNil(t, *patch.Description)
				assert.Equal(t, "now with detail", **patch.Description)
			},
		},
		{
			name: "empty string is a value, not a clear",
			body: `{"description": ""}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.Description)
				require.NotNil(t, *patch.Description)
				assert.Equal(t, "", **patch.Description)
			},
		},
		{
			name: "explicit null clears reviewer",
			body: `{"reviewer": null}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.Reviewer)
				assert.Nil(t, *patch.Reviewer)
				assert.Nil(t, patch.Description)
			},
		},
		{
			name: "numeric nullables follow the same rule",
			body: `{"estimate_hours": 4, "buffer_hours": null}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.EstimateHours)
				require.NotNil(t, *patch.EstimateHours)
				assert.InDelta(t, 4, **patch.EstimateHours, 0.0001)

				require.NotNil(t, patch.BufferHours)
				assert.Nil(t, *patch.BufferHours)

				assert.Nil(t, patch.SpentHours, "untouched nullable stays nil")
			},
		},
		{
			name: "explicit null clears sprint_id",
			body: `{"sprint_id": null}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.SprintID)
				assert.Nil(t, *patch.SprintID)
			},
		},
		{
			name: "sprint_id value is parsed into a UUID",
			body: `{"sprint_id": "` + sprintID.String() + `"}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.SprintID)
				require.NotNil(t, *patch.SprintID)
				assert.Equal(t, sprintID, **patch.SprintID)
			},
		},
		{
			name: "blank sprint_id clears the link",
			body: `{"sprint_id": ""}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.SprintID)
				assert.Nil(t, *patch.SprintID)
			},
		},
		{
			name: "explicit null clears due_date",
			body: `{"due_date": null}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.DueDate)
				assert.Nil(t, *patch.DueDate)
			},
		},
		{
			name: "due_date value is parsed",
			body: `{"due_date": "2026-08-15"}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.DueDate)
				require.NotNil(t, *patch.DueDate)
				assert.Equal(t, time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC), (**patch.DueDate).UTC())
			},
		},
		{
			name: "non-nullable enums are plain pointers",
			body: `{"status": "IN_REVIEW", "priority": "URGENT"}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				require.NotNil(t, patch.Status)
				assert.Equal(t, models.StatusInReview, *patch.Status)
				require.NotNil(t, patch.Priority)
				assert.Equal(t, models.PriorityUrgent, *patch.Priority)
			},
		},
		{
			name: "service-owned timestamps are never set from a request",
			body: `{"started_at": "2026-07-01T00:00:00Z", "completed_at": "2026-07-02T00:00:00Z", "position": 12}`,
			assert: func(t *testing.T, patch models.TaskPatch) {
				assert.Nil(t, patch.StartedAt)
				assert.Nil(t, patch.CompletedAt)
				assert.Nil(t, patch.Position)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockTask := new(MockTaskService)
			updated := sampleTask(userID, taskID)

			var got models.TaskPatch
			mockTask.On("UpdateTask", mock.Anything, userID, taskID, mock.Anything).
				Run(func(args mock.Arguments) { got = args.Get(3).(models.TaskPatch) }).
				Return(updated, nil)

			r := setupTaskRouter(mockTask, nil, nil, userID)

			w := doRequest(r, http.MethodPatch, "/tasks/"+taskID.String(), []byte(tc.body))

			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, taskID, decodeTask(t, w.Body.Bytes()).ID)
			tc.assert(t, got)
			mockTask.AssertExpectations(t)
		})
	}
}

func TestTaskHandler_UpdateTask_Errors(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	tests := []struct {
		name       string
		path       string
		body       []byte
		svcErr     error
		expectCall bool
		wantStatus int
		wantBody   string
	}{
		{
			name:       "malformed JSON is a 400, never a 500",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{"title": "half`),
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid task payload",
		},
		{
			name:       "wrong field type is a 400",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{"description": 42}`),
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid UUID in the path is a 400",
			path:       "/tasks/not-a-uuid",
			body:       []byte(`{"title":"x"}`),
			wantStatus: http.StatusBadRequest,
			wantBody:   "Invalid id format",
		},
		{
			name:       "null title is rejected",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{"title": null}`),
			wantStatus: http.StatusBadRequest,
			wantBody:   apperrors.ErrEmptyTaskTitle.Error(),
		},
		{
			name:       "null status is rejected",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{"status": null}`),
			wantStatus: http.StatusBadRequest,
			wantBody:   apperrors.ErrInvalidTaskStatus.Error(),
		},
		{
			name:       "null priority is rejected",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{"priority": null}`),
			wantStatus: http.StatusBadRequest,
			wantBody:   apperrors.ErrInvalidTaskPriority.Error(),
		},
		{
			name:       "unparseable sprint_id is rejected",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{"sprint_id": "nope"}`),
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid sprint_id",
		},
		{
			name:       "empty patch surfaces the service error as a 400",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{}`),
			svcErr:     apperrors.ErrNothingToUpdate,
			expectCall: true,
			wantStatus: http.StatusBadRequest,
			wantBody:   apperrors.ErrNothingToUpdate.Error(),
		},
		{
			name:       "missing task is a 404",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{"title":"x"}`),
			svcErr:     apperrors.ErrTaskNotFound,
			expectCall: true,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "reviewer requirement from the service is a 400",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{"status":"IN_REVIEW"}`),
			svcErr:     apperrors.ErrReviewerRequired,
			expectCall: true,
			wantStatus: http.StatusBadRequest,
			wantBody:   apperrors.ErrReviewerRequired.Error(),
		},
		{
			name:       "unknown service error is a 500",
			path:       "/tasks/" + taskID.String(),
			body:       []byte(`{"title":"x"}`),
			svcErr:     assert.AnError,
			expectCall: true,
			wantStatus: http.StatusInternalServerError,
			wantBody:   "Failed to update task",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockTask := new(MockTaskService)
			if tc.expectCall {
				mockTask.On("UpdateTask", mock.Anything, userID, taskID, mock.Anything).
					Return(nil, tc.svcErr)
			}

			r := setupTaskRouter(mockTask, nil, nil, userID)

			w := doRequest(r, http.MethodPatch, tc.path, tc.body)

			assert.Equal(t, tc.wantStatus, w.Code)
			if tc.wantBody != "" {
				assert.Contains(t, w.Body.String(), tc.wantBody)
			}
			if !tc.expectCall {
				mockTask.AssertNotCalled(t, "UpdateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
			mockTask.AssertExpectations(t)
		})
	}
}

// ---------------------------------------------------------------------------
// PATCH /tasks/:id/status
// ---------------------------------------------------------------------------

func TestTaskHandler_UpdateTaskStatus(t *testing.T) {
	userID := uuid.New()
	taskUUID := uuid.New()
	taskID := taskUUID.String()

	t.Run("Success - 200 OK", func(t *testing.T) {
		mockTask := new(MockTaskService)
		mockTask.On("UpdateTaskStatus", mock.Anything, taskID, userID, models.TaskStatus("DONE"), (*string)(nil)).Return(nil)
		mockTask.On("GetTask", mock.Anything, userID, taskUUID).Return(sampleTask(userID, taskUUID), nil)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		body, _ := json.Marshal(map[string]string{
			"status": "DONE",
		})

		w := doRequest(r, http.MethodPatch, "/tasks/"+taskID+"/status", body)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Task status updated successfully")
		mockTask.AssertExpectations(t)
	})

	t.Run("Invalid Task UUID Format - 400 Bad Request", func(t *testing.T) {
		mockTask := new(MockTaskService)
		r := setupTaskRouter(mockTask, nil, nil, userID)

		body, _ := json.Marshal(map[string]string{"status": "DONE"})

		w := doRequest(r, http.MethodPatch, "/tasks/invalid-uuid-format/status", body)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid id format")
		mockTask.AssertNotCalled(t, "UpdateTaskStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})

	t.Run("Task Not Found - 404 Not Found", func(t *testing.T) {
		mockTask := new(MockTaskService)
		mockTask.On("UpdateTaskStatus", mock.Anything, taskID, userID, models.TaskStatus("DONE"), (*string)(nil)).
			Return(apperrors.ErrTaskNotFound)

		r := setupTaskRouter(mockTask, nil, nil, userID)

		body, _ := json.Marshal(map[string]string{"status": "DONE"})

		w := doRequest(r, http.MethodPatch, "/tasks/"+taskID+"/status", body)

		assert.Equal(t, http.StatusNotFound, w.Code)
		mockTask.AssertNotCalled(t, "GetTask", mock.Anything, mock.Anything, mock.Anything)
		mockTask.AssertExpectations(t)
	})

	t.Run("Reviewer handling", func(t *testing.T) {
		tests := []struct {
			name         string
			body         string
			svcErr       error
			wantStatus   int
			wantSent     models.TaskStatus
			wantReviewer *string
			wantBody     string
		}{
			{
				name:         "IN_REVIEW with a reviewer is forwarded",
				body:         `{"status":"IN_REVIEW","reviewer":"alice"}`,
				wantStatus:   http.StatusOK,
				wantSent:     models.StatusInReview,
				wantReviewer: new("alice"),
				wantBody:     "Task status updated successfully",
			},
			{
				name:         "absent reviewer is forwarded as nil",
				body:         `{"status":"IN_PROGRESS"}`,
				wantStatus:   http.StatusOK,
				wantSent:     models.StatusInProgress,
				wantReviewer: nil,
			},
			{
				name:         "explicit null reviewer is forwarded as nil",
				body:         `{"status":"TODO","reviewer":null}`,
				wantStatus:   http.StatusOK,
				wantSent:     models.StatusTodo,
				wantReviewer: nil,
			},
			{
				name:         "missing reviewer for IN_REVIEW is a 400",
				body:         `{"status":"IN_REVIEW"}`,
				svcErr:       apperrors.ErrReviewerRequired,
				wantStatus:   http.StatusBadRequest,
				wantSent:     models.StatusInReview,
				wantReviewer: nil,
				wantBody:     apperrors.ErrReviewerRequired.Error(),
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				mockTask := new(MockTaskService)

				var gotStatus models.TaskStatus
				var gotReviewer *string
				mockTask.On("UpdateTaskStatus", mock.Anything, taskID, userID, mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						gotStatus = args.Get(3).(models.TaskStatus)
						gotReviewer = args.Get(4).(*string)
					}).
					Return(tc.svcErr)

				if tc.svcErr == nil {
					mockTask.On("GetTask", mock.Anything, userID, taskUUID).Return(sampleTask(userID, taskUUID), nil)
				}

				r := setupTaskRouter(mockTask, nil, nil, userID)

				w := doRequest(r, http.MethodPatch, "/tasks/"+taskID+"/status", []byte(tc.body))

				assert.Equal(t, tc.wantStatus, w.Code, w.Body.String())
				assert.Equal(t, tc.wantSent, gotStatus)
				assert.Equal(t, tc.wantReviewer, gotReviewer)
				if tc.wantBody != "" {
					assert.Contains(t, w.Body.String(), tc.wantBody)
				}
				mockTask.AssertExpectations(t)
			})
		}
	})

	t.Run("Bad payloads - 400 Bad Request", func(t *testing.T) {
		tests := []struct {
			name string
			body []byte
		}{
			{name: "malformed JSON", body: []byte(`{"status": "DON`)},
			{name: "missing status", body: []byte(`{"reviewer":"alice"}`)},
			{name: "empty status", body: []byte(`{"status":""}`)},
			{name: "wrong reviewer type", body: []byte(`{"status":"DONE","reviewer":7}`)},
			{name: "no body at all", body: nil},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				mockTask := new(MockTaskService)
				r := setupTaskRouter(mockTask, nil, nil, userID)

				w := doRequest(r, http.MethodPatch, "/tasks/"+taskID+"/status", tc.body)

				assert.Equal(t, http.StatusBadRequest, w.Code)
				mockTask.AssertNotCalled(t, "UpdateTaskStatus", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	})
}

// ---------------------------------------------------------------------------
// PATCH /tasks/:id/move
// ---------------------------------------------------------------------------

func TestTaskHandler_MoveTask(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()
	afterID := uuid.New()
	beforeID := uuid.New()

	t.Run("Neighbour forwarding", func(t *testing.T) {
		tests := []struct {
			name       string
			body       string
			wantStatus models.TaskStatus
			wantAfter  *uuid.UUID
			wantBefore *uuid.UUID
		}{
			{
				name:       "both neighbours are forwarded",
				body:       `{"status":"IN_PROGRESS","after_id":"` + afterID.String() + `","before_id":"` + beforeID.String() + `"}`,
				wantStatus: models.StatusInProgress,
				wantAfter:  &afterID,
				wantBefore: &beforeID,
			},
			{
				name:       "absent neighbours arrive as nil",
				body:       `{"status":"TODO"}`,
				wantStatus: models.StatusTodo,
				wantAfter:  nil,
				wantBefore: nil,
			},
			{
				name:       "explicit nulls arrive as nil",
				body:       `{"status":"DONE","after_id":null,"before_id":null}`,
				wantStatus: models.StatusDone,
				wantAfter:  nil,
				wantBefore: nil,
			},
			{
				name:       "top of a column: only before_id",
				body:       `{"status":"IN_REVIEW","before_id":"` + beforeID.String() + `"}`,
				wantStatus: models.StatusInReview,
				wantAfter:  nil,
				wantBefore: &beforeID,
			},
			{
				name:       "bottom of a column: only after_id",
				body:       `{"status":"IN_REVIEW","after_id":"` + afterID.String() + `","before_id":null}`,
				wantStatus: models.StatusInReview,
				wantAfter:  &afterID,
				wantBefore: nil,
			},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				mockTask := new(MockTaskService)

				moved := &models.Task{
					ID:       taskID,
					UserID:   userID,
					Title:    "Moved task",
					Status:   tc.wantStatus,
					Priority: models.PriorityMedium,
					Position: 1536,
				}

				var gotStatus models.TaskStatus
				var gotAfter, gotBefore *uuid.UUID
				mockTask.On("MoveTask", mock.Anything, userID, taskID, mock.Anything, mock.Anything, mock.Anything).
					Run(func(args mock.Arguments) {
						gotStatus = args.Get(3).(models.TaskStatus)
						gotAfter, _ = args.Get(4).(*uuid.UUID)
						gotBefore, _ = args.Get(5).(*uuid.UUID)
					}).
					Return(moved, nil)

				r := setupTaskRouter(mockTask, nil, nil, userID)

				w := doRequest(r, http.MethodPatch, "/tasks/"+taskID.String()+"/move", []byte(tc.body))

				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				assert.Equal(t, tc.wantStatus, gotStatus)
				assert.Equal(t, tc.wantAfter, gotAfter)
				assert.Equal(t, tc.wantBefore, gotBefore)

				got := decodeTask(t, w.Body.Bytes())
				assert.Equal(t, taskID, got.ID)
				assert.Equal(t, tc.wantStatus, got.Status)
				assert.InDelta(t, 1536, got.Position, 0.0001)
				mockTask.AssertExpectations(t)
			})
		}
	})

	t.Run("Bad payloads - 400 Bad Request", func(t *testing.T) {
		tests := []struct {
			name string
			path string
			body []byte
		}{
			{name: "malformed JSON", path: "/tasks/" + taskID.String() + "/move", body: []byte(`{"status":`)},
			{name: "missing status", path: "/tasks/" + taskID.String() + "/move", body: []byte(`{"after_id":"` + afterID.String() + `"}`)},
			{name: "unparseable after_id", path: "/tasks/" + taskID.String() + "/move", body: []byte(`{"status":"TODO","after_id":"nope"}`)},
			{name: "invalid UUID in the path", path: "/tasks/not-a-uuid/move", body: []byte(`{"status":"TODO"}`)},
			{name: "no body at all", path: "/tasks/" + taskID.String() + "/move", body: nil},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				mockTask := new(MockTaskService)
				r := setupTaskRouter(mockTask, nil, nil, userID)

				w := doRequest(r, http.MethodPatch, tc.path, tc.body)

				assert.Equal(t, http.StatusBadRequest, w.Code)
				mockTask.AssertNotCalled(t, "MoveTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("Service errors", func(t *testing.T) {
		tests := []struct {
			name       string
			svcErr     error
			wantStatus int
		}{
			{name: "task not found is a 404", svcErr: apperrors.ErrTaskNotFound, wantStatus: http.StatusNotFound},
			{name: "invalid status is a 400", svcErr: apperrors.ErrInvalidTaskStatus, wantStatus: http.StatusBadRequest},
			{name: "reviewer required is a 400", svcErr: apperrors.ErrReviewerRequired, wantStatus: http.StatusBadRequest},
			{name: "unknown error is a 500", svcErr: assert.AnError, wantStatus: http.StatusInternalServerError},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				mockTask := new(MockTaskService)
				mockTask.On("MoveTask", mock.Anything, userID, taskID, models.StatusInReview, mock.Anything, mock.Anything).
					Return(nil, tc.svcErr)

				r := setupTaskRouter(mockTask, nil, nil, userID)

				w := doRequest(r, http.MethodPatch, "/tasks/"+taskID.String()+"/move", []byte(`{"status":"IN_REVIEW"}`))

				assert.Equal(t, tc.wantStatus, w.Code)
				mockTask.AssertExpectations(t)
			})
		}
	})

	t.Run("Unauthorized - 401 Missing Context UserID", func(t *testing.T) {
		mockTask := new(MockTaskService)
		r := setupTaskRouter(mockTask, nil, nil, uuid.Nil)

		w := doRequest(r, http.MethodPatch, "/tasks/"+taskID.String()+"/move", []byte(`{"status":"TODO"}`))

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		mockTask.AssertNotCalled(t, "MoveTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	})
}

// ---------------------------------------------------------------------------
// DELETE /tasks/:id
// ---------------------------------------------------------------------------

func TestTaskHandler_DeleteTask(t *testing.T) {
	userID := uuid.New()
	taskID := uuid.New()

	tests := []struct {
		name       string
		path       string
		svcErr     error
		expectCall bool
		wantStatus int
	}{
		{
			name:       "success is a 204 with an empty body",
			path:       "/tasks/" + taskID.String(),
			expectCall: true,
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "missing task is a 404",
			path:       "/tasks/" + taskID.String(),
			svcErr:     apperrors.ErrTaskNotFound,
			expectCall: true,
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "unknown service error is a 500",
			path:       "/tasks/" + taskID.String(),
			svcErr:     assert.AnError,
			expectCall: true,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "invalid UUID in the path is a 400",
			path:       "/tasks/not-a-uuid",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockTask := new(MockTaskService)
			if tc.expectCall {
				mockTask.On("DeleteTask", mock.Anything, userID, taskID).Return(tc.svcErr)
			}

			r := setupTaskRouter(mockTask, nil, nil, userID)

			w := doRequest(r, http.MethodDelete, tc.path, nil)

			assert.Equal(t, tc.wantStatus, w.Code)
			if tc.wantStatus == http.StatusNoContent {
				assert.Empty(t, w.Body.Bytes(), "204 must carry no body")
			}
			if !tc.expectCall {
				mockTask.AssertNotCalled(t, "DeleteTask", mock.Anything, mock.Anything, mock.Anything)
			}
			mockTask.AssertExpectations(t)
		})
	}
}

// ---------------------------------------------------------------------------
// POST /tasks/share
// ---------------------------------------------------------------------------

func TestTaskHandler_ShareTasks(t *testing.T) {
	userID := uuid.New()
	senderEmail := "sender@example.com"
	recipientEmail := "friend@example.com"

	t.Run("Success - 200 OK", func(t *testing.T) {
		mockAuth := new(MockAuthService)
		mockEmail := new(MockEmailService)

		user := &models.User{ID: userID, Email: senderEmail}

		mockAuth.On("GetUserByID", mock.Anything, userID).Return(user, nil)
		mockEmail.On("ShareTasks", mock.Anything, recipientEmail, senderEmail, "http://localhost:3000/dashboard").Return(nil)

		r := setupTaskRouter(nil, mockEmail, mockAuth, userID)

		body, _ := json.Marshal(map[string]string{
			"recipient_email": recipientEmail,
		})

		w := doRequest(r, http.MethodPost, "/tasks/share", body)

		assert.Equal(t, http.StatusOK, w.Code)
		mockAuth.AssertExpectations(t)
		mockEmail.AssertExpectations(t)
	})

	t.Run("Bad payloads - 400 Bad Request", func(t *testing.T) {
		tests := []struct {
			name string
			body []byte
		}{
			{name: "malformed JSON", body: []byte(`{"recipient_email":`)},
			{name: "missing recipient", body: []byte(`{}`)},
			{name: "not an email", body: []byte(`{"recipient_email":"nope"}`)},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				mockAuth := new(MockAuthService)
				mockEmail := new(MockEmailService)

				r := setupTaskRouter(nil, mockEmail, mockAuth, userID)

				w := doRequest(r, http.MethodPost, "/tasks/share", tc.body)

				assert.Equal(t, http.StatusBadRequest, w.Code)
				assert.Contains(t, w.Body.String(), apperrors.ErrEmptyRecipient.Error())
				mockEmail.AssertNotCalled(t, "ShareTasks", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	})
}
