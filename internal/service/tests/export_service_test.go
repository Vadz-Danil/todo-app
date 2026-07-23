package service_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/config"
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
// mocks and stubs
// ---------------------------------------------------------------------------

type exMockExportRepo struct {
	mock.Mock
}

var _ repository.ExportRepository = (*exMockExportRepo)(nil)

func (m *exMockExportRepo) CreateTarget(ctx context.Context, target *models.ExportTarget) error {
	return m.Called(ctx, target).Error(0)
}

func (m *exMockExportRepo) GetTarget(ctx context.Context, userID, targetID uuid.UUID) (*models.ExportTarget, error) {
	args := m.Called(ctx, userID, targetID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.ExportTarget), args.Error(1)
}

func (m *exMockExportRepo) ListTargets(ctx context.Context, userID uuid.UUID) ([]models.ExportTarget, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.ExportTarget), args.Error(1)
}

func (m *exMockExportRepo) UpdateTarget(ctx context.Context, target *models.ExportTarget) error {
	return m.Called(ctx, target).Error(0)
}

func (m *exMockExportRepo) DeleteTarget(ctx context.Context, userID, targetID uuid.UUID) error {
	return m.Called(ctx, userID, targetID).Error(0)
}

func (m *exMockExportRepo) TouchTarget(ctx context.Context, userID, targetID uuid.UUID, statusCode *int, errMsg *string, sentAt time.Time) error {
	return m.Called(ctx, userID, targetID, statusCode, errMsg, sentAt).Error(0)
}

func (m *exMockExportRepo) RecordDelivery(ctx context.Context, delivery *models.ExportDelivery) error {
	return m.Called(ctx, delivery).Error(0)
}

func (m *exMockExportRepo) ListDeliveries(ctx context.Context, userID uuid.UUID, limit int) ([]models.ExportDelivery, error) {
	args := m.Called(ctx, userID, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.ExportDelivery), args.Error(1)
}

// exStubAnalytics is a hand-rolled stub: the export service only ever calls
// Dashboard, so the rest of the interface is inert.
type exStubAnalytics struct {
	dashboard *models.Dashboard
	err       error
	calls     int
}

var _ service.Analytics = (*exStubAnalytics)(nil)

func (s *exStubAnalytics) ResolveQuery(string, string, string, string, string) (models.AnalyticsQuery, error) {
	return models.AnalyticsQuery{}, nil
}

func (s *exStubAnalytics) Dashboard(context.Context, uuid.UUID, models.AnalyticsQuery) (*models.Dashboard, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.dashboard, nil
}

type exStubTasks struct {
	tasks      []models.Task
	err        error
	calls      int
	lastFilter models.TaskFilter
}

var _ service.Task = (*exStubTasks)(nil)

func (s *exStubTasks) CreateTask(context.Context, uuid.UUID, models.TaskCreate) (*models.Task, error) {
	return nil, nil
}

func (s *exStubTasks) GetTasks(_ context.Context, _ uuid.UUID, filter models.TaskFilter) ([]models.Task, error) {
	s.calls++
	s.lastFilter = filter
	if s.err != nil {
		return nil, s.err
	}
	return s.tasks, nil
}

func (s *exStubTasks) GetTask(context.Context, uuid.UUID, uuid.UUID) (*models.Task, error) {
	return nil, nil
}

func (s *exStubTasks) UpdateTask(context.Context, uuid.UUID, uuid.UUID, models.TaskPatch) (*models.Task, error) {
	return nil, nil
}

func (s *exStubTasks) UpdateTaskStatus(context.Context, string, uuid.UUID, models.TaskStatus, *string) error {
	return nil
}

func (s *exStubTasks) MoveTask(context.Context, uuid.UUID, uuid.UUID, models.TaskStatus, *uuid.UUID, *uuid.UUID) (*models.Task, error) {
	return nil, nil
}

func (s *exStubTasks) DeleteTask(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (s *exStubTasks) CreateTasksFromPlan(context.Context, uuid.UUID, *uuid.UUID, *models.SprintPlan) ([]models.Task, error) {
	return nil, nil
}

type exStubSprints struct {
	sprints []models.Sprint
	err     error
	calls   int
}

var _ service.Sprint = (*exStubSprints)(nil)

func (s *exStubSprints) Create(context.Context, uuid.UUID, service.SprintInput) (*models.Sprint, error) {
	return nil, nil
}

func (s *exStubSprints) Get(context.Context, uuid.UUID, uuid.UUID) (*models.Sprint, error) {
	return nil, nil
}

func (s *exStubSprints) List(context.Context, uuid.UUID) ([]models.Sprint, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.sprints, nil
}

func (s *exStubSprints) Update(context.Context, uuid.UUID, uuid.UUID, service.SprintInput) (*models.Sprint, error) {
	return nil, nil
}

func (s *exStubSprints) Delete(context.Context, uuid.UUID, uuid.UUID) error { return nil }

// ---------------------------------------------------------------------------
// fixtures
// ---------------------------------------------------------------------------

type exFixture struct {
	repo      *exMockExportRepo
	analytics *exStubAnalytics
	tasks     *exStubTasks
	sprints   *exStubSprints
	svc       *service.ExportService
}

func exNewFixture(cfg config.ExportConfig) *exFixture {
	f := &exFixture{
		repo:      new(exMockExportRepo),
		analytics: &exStubAnalytics{dashboard: exDashboard()},
		tasks:     &exStubTasks{tasks: exTasks()},
		sprints:   &exStubSprints{sprints: exSprints()},
	}
	f.svc = service.NewExportService(f.repo, f.analytics, f.tasks, f.sprints, cfg, zap.NewNop())
	return f
}

func exConfig() config.ExportConfig {
	return config.ExportConfig{
		Timeout:        5 * time.Second,
		MaxRetries:     0,
		AllowPrivate:   true,
		MaxPayloadSize: 8 << 20,
	}
}

func exUser() *models.User {
	return &models.User{
		ID:        uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		Email:     "owner@example.com",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func exQuery() models.AnalyticsQuery {
	return models.AnalyticsQuery{
		Period:      models.PeriodWeek,
		From:        time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		To:          time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		Granularity: models.GranularityDay,
		Location:    time.UTC,
	}
}

func exDashboard() *models.Dashboard {
	return &models.Dashboard{
		Range: models.RangeInfo{
			Period:      models.PeriodWeek,
			From:        time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
			To:          time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
			Granularity: models.GranularityDay,
			Timezone:    "UTC",
			Label:       "Last week",
			Days:        7,
		},
		Totals:      models.Totals{TotalTasks: 3, CompletedInRange: 1, OpenNow: 2},
		GeneratedAt: time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
	}
}

func exTasks() []models.Task {
	return []models.Task{
		{ID: uuid.New(), Title: "write tests", Status: models.StatusInProgress, Priority: models.PriorityHigh},
		{ID: uuid.New(), Title: "ship it", Status: models.StatusDone, Priority: models.PriorityMedium},
		{ID: uuid.New(), Title: "review PR", Status: models.StatusInReview, Priority: models.PriorityLow},
	}
}

func exSprints() []models.Sprint {
	return []models.Sprint{
		{ID: uuid.New(), Name: "Sprint 42", Status: models.SprintActive},
	}
}

func exSummary() *models.AISummary {
	return &models.AISummary{
		ID:      uuid.New(),
		Period:  "week",
		Content: models.AISummaryContent{Headline: "solid week", Summary: "you shipped things", Trend: "UP", Score: 80},
	}
}

// exCapturedRequest is one delivery as the receiving endpoint saw it.
type exCapturedRequest struct {
	method string
	header http.Header
	body   []byte
}

type exReceiver struct {
	mu       sync.Mutex
	requests []exCapturedRequest
}

func (r *exReceiver) add(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, exCapturedRequest{
		method: req.Method,
		header: req.Header.Clone(),
		body:   body,
	})
}

func (r *exReceiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func (r *exReceiver) last() exCapturedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests[len(r.requests)-1]
}

// exStartReceiver runs a webhook endpoint that answers with the given statuses
// in order, repeating the last one once the list is exhausted.
func exStartReceiver(t *testing.T, statuses ...int) (string, *exReceiver) {
	t.Helper()

	rec := &exReceiver{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.add(r)

		idx := rec.count() - 1
		status := http.StatusOK
		if len(statuses) > 0 {
			if idx >= len(statuses) {
				idx = len(statuses) - 1
			}
			status = statuses[idx]
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"received":true}`))
	}))
	t.Cleanup(srv.Close)

	return srv.URL + "/hook", rec
}

// exExpectSignature recomputes the HMAC the service should have produced from
// exactly what the endpoint received.
func exExpectSignature(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "."))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func exStr(s string) *string { return &s }

// ---------------------------------------------------------------------------
// validateURL / SSRF guard
// ---------------------------------------------------------------------------

func TestExportService_ValidateURL_Malformed(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{name: "empty", url: ""},
		{name: "whitespace only", url: "   \t\n "},
		{name: "ftp scheme", url: "ftp://example.com/hook"},
		{name: "file scheme", url: "file:///etc/passwd"},
		{name: "gopher scheme", url: "gopher://example.com:70/_hook"},
		{name: "javascript scheme", url: "javascript:alert(1)"},
		{name: "data scheme", url: "data:text/plain,hello"},
		{name: "scheme relative", url: "//example.com/hook"},
		{name: "no scheme at all", url: "example.com/hook"},
		{name: "absolute path only", url: "/hook"},
		{name: "hostless http", url: "http:///hook"},
		{name: "hostless https", url: "https://"},
		{name: "port without host", url: "http://:8080/hook"},
		{name: "unparseable host", url: "http://exa mple.com/hook"},
	}

	// A malformed URL is malformed regardless of the private-network policy.
	for _, allowPrivate := range []bool{false, true} {
		t.Run("allow_private="+strconv.FormatBool(allowPrivate), func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					cfg := exConfig()
					cfg.AllowPrivate = allowPrivate
					f := exNewFixture(cfg)
					f.repo.On("CreateTarget", mock.Anything, mock.Anything).Return(nil).Maybe()

					target, err := f.svc.CreateTarget(context.Background(), exUser().ID,
						service.TargetInput{Name: exStr("hook"), URL: exStr(tc.url)})

					require.Error(t, err)
					assert.Nil(t, target)
					assert.ErrorIs(t, err, apperrors.ErrInvalidExportURL)
					assert.NotErrorIs(t, err, apperrors.ErrBlockedExportURL)
					f.repo.AssertNotCalled(t, "CreateTarget", mock.Anything, mock.Anything)
				})
			}
		})
	}
}

func TestExportService_ValidateURL_PrivateNetworks(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{name: "loopback ipv4", url: "http://127.0.0.1:8080/hook"},
		{name: "loopback ipv4 https", url: "https://127.0.0.1/hook"},
		{name: "loopback ipv4 alternate", url: "http://127.53.12.9/hook"},
		{name: "loopback hostname", url: "http://localhost:9000/hook"},
		{name: "loopback ipv6", url: "http://[::1]:8080/hook"},
		{name: "private 10.x", url: "http://10.0.0.5/hook"},
		{name: "private 10.x high", url: "http://10.255.255.254:3000/hook"},
		{name: "private 172.16.x", url: "http://172.16.0.1/hook"},
		{name: "private 192.168.x", url: "http://192.168.1.10/hook"},
		{name: "link local metadata", url: "http://169.254.169.254/latest/meta-data/iam/security-credentials/"},
		{name: "link local ipv6", url: "http://[fe80::1]/hook"},
		{name: "unspecified", url: "http://0.0.0.0:8080/hook"},
		{name: "multicast", url: "http://224.0.0.1/hook"},
	}

	t.Run("blocked when AllowPrivate is false", func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := exConfig()
				cfg.AllowPrivate = false
				f := exNewFixture(cfg)
				f.repo.On("CreateTarget", mock.Anything, mock.Anything).Return(nil).Maybe()

				target, err := f.svc.CreateTarget(context.Background(), exUser().ID,
					service.TargetInput{Name: exStr("hook"), URL: exStr(tc.url)})

				require.Error(t, err)
				assert.Nil(t, target)
				assert.ErrorIs(t, err, apperrors.ErrBlockedExportURL,
					"%s must be refused by the SSRF guard", tc.url)
				f.repo.AssertNotCalled(t, "CreateTarget", mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("accepted when AllowPrivate is true", func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := exConfig()
				cfg.AllowPrivate = true
				f := exNewFixture(cfg)
				f.repo.On("CreateTarget", mock.Anything, mock.Anything).Return(nil).Once()

				target, err := f.svc.CreateTarget(context.Background(), exUser().ID,
					service.TargetInput{Name: exStr("hook"), URL: exStr(tc.url)})

				require.NoError(t, err)
				require.NotNil(t, target)
				assert.Equal(t, tc.url, target.URL)
				f.repo.AssertExpectations(t)
			})
		}
	})
}

func TestExportService_ValidateURL_PublicAddressesAreAllowed(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{name: "public ipv4", url: "https://93.184.216.34/hook"},
		{name: "public ipv4 with port", url: "https://1.1.1.1:8443/hook?source=taskflow"},
		{name: "public ipv4 plain http", url: "http://8.8.8.8/webhook"},
		{name: "public ipv6", url: "https://[2606:4700:4700::1111]/hook"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := exConfig()
			cfg.AllowPrivate = false
			f := exNewFixture(cfg)
			f.repo.On("CreateTarget", mock.Anything, mock.Anything).Return(nil).Once()

			target, err := f.svc.CreateTarget(context.Background(), exUser().ID,
				service.TargetInput{Name: exStr("hook"), URL: exStr(tc.url), Secret: exStr("s3cret")})

			require.NoError(t, err)
			require.NotNil(t, target)
			assert.Equal(t, tc.url, target.URL)
			assert.True(t, target.HasSecret)
			assert.Empty(t, target.Secret, "the secret must never be returned to the caller")
			f.repo.AssertExpectations(t)
		})
	}
}

func TestExportService_Push_RejectsBlockedAdHocURL(t *testing.T) {
	cfg := exConfig()
	cfg.AllowPrivate = false
	f := exNewFixture(cfg)

	delivery, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
		URL:   "http://169.254.169.254/latest/meta-data/",
		Kind:  models.ExportFull,
		Query: exQuery(),
	})

	require.Error(t, err)
	assert.Nil(t, delivery)
	assert.ErrorIs(t, err, apperrors.ErrBlockedExportURL)
	assert.Zero(t, f.analytics.calls, "a blocked URL must be refused before any work happens")
	f.repo.AssertNotCalled(t, "RecordDelivery", mock.Anything, mock.Anything)
}

// ---------------------------------------------------------------------------
// Push: transport contract
// ---------------------------------------------------------------------------

func TestExportService_Push_RequestContract(t *testing.T) {
	t.Run("signs the payload and sets the protocol headers", func(t *testing.T) {
		const secret = "whsec_super_secret_value"

		endpoint, receiver := exStartReceiver(t, http.StatusOK)
		f := exNewFixture(exConfig())
		f.repo.On("RecordDelivery", mock.Anything, mock.Anything).Return(nil).Once()

		before := time.Now().Unix()
		delivery, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
			URL:    endpoint,
			Secret: secret,
			Kind:   models.ExportAnalytics,
			Query:  exQuery(),
		})
		after := time.Now().Unix()

		require.NoError(t, err)
		require.NotNil(t, delivery)
		require.Equal(t, 1, receiver.count())

		got := receiver.last()
		assert.Equal(t, http.MethodPost, got.method)
		assert.Equal(t, "application/json", got.header.Get("Content-Type"))
		assert.Equal(t, "TaskFlow-Exporter/1", got.header.Get("User-Agent"))
		assert.Equal(t, string(models.ExportAnalytics), got.header.Get("X-TaskFlow-Event"))

		timestamp := got.header.Get("X-TaskFlow-Timestamp")
		require.NotEmpty(t, timestamp)
		unix, convErr := strconv.ParseInt(timestamp, 10, 64)
		require.NoError(t, convErr, "the timestamp must be unix seconds")
		assert.GreaterOrEqual(t, unix, before)
		assert.LessOrEqual(t, unix, after)

		assert.Equal(t, exExpectSignature(secret, timestamp, got.body), got.header.Get("X-TaskFlow-Signature"),
			"signature must be sha256= over \"<timestamp>.<body>\"")

		// The signed bytes really are the envelope.
		var envelope models.ExportEnvelope
		require.NoError(t, json.Unmarshal(got.body, &envelope))
		assert.Equal(t, models.ExportSchema, envelope.Schema)
		assert.Equal(t, models.ExportAnalytics, envelope.Kind)
		assert.Equal(t, len(got.body), delivery.PayloadSize)
	})

	t.Run("no secret means no signature header", func(t *testing.T) {
		endpoint, receiver := exStartReceiver(t, http.StatusAccepted)
		f := exNewFixture(exConfig())
		f.repo.On("RecordDelivery", mock.Anything, mock.Anything).Return(nil).Once()

		_, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
			URL:   endpoint,
			Kind:  models.ExportTasks,
			Query: exQuery(),
		})

		require.NoError(t, err)
		require.Equal(t, 1, receiver.count())

		got := receiver.last()
		assert.Empty(t, got.header.Get("X-TaskFlow-Signature"))
		assert.NotEmpty(t, got.header.Get("X-TaskFlow-Timestamp"), "the timestamp is sent even unsigned")
	})

	t.Run("blank secret is treated as no secret", func(t *testing.T) {
		endpoint, receiver := exStartReceiver(t, http.StatusOK)
		f := exNewFixture(exConfig())
		f.repo.On("RecordDelivery", mock.Anything, mock.Anything).Return(nil).Once()

		_, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
			URL:    endpoint,
			Secret: "   ",
			Kind:   models.ExportTasks,
			Query:  exQuery(),
		})

		require.NoError(t, err)
		assert.Empty(t, receiver.last().header.Get("X-TaskFlow-Signature"))
	})

	t.Run("custom headers are forwarded but cannot hijack the contract", func(t *testing.T) {
		const secret = "whsec_do_not_spoof_me"

		endpoint, receiver := exStartReceiver(t, http.StatusOK)
		f := exNewFixture(exConfig())
		f.repo.On("RecordDelivery", mock.Anything, mock.Anything).Return(nil).Once()

		_, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
			URL:    endpoint,
			Secret: secret,
			Kind:   models.ExportSprints,
			Query:  exQuery(),
			Headers: map[string]string{
				"X-Custom-Token":       "abc123",
				"Authorization":        "Bearer downstream-token",
				"Content-Type":         "text/plain",
				"content-type":         "application/xml",
				"X-TaskFlow-Event":     "SPOOFED",
				"X-TaskFlow-Timestamp": "0",
				"X-TaskFlow-Signature": "sha256=deadbeef",
				"x-taskflow-signature": "sha256=deadbeef",
				"":                     "ignored",
			},
		})

		require.NoError(t, err)
		require.Equal(t, 1, receiver.count())

		got := receiver.last()
		assert.Equal(t, "abc123", got.header.Get("X-Custom-Token"), "harmless custom headers are forwarded")
		assert.Equal(t, "Bearer downstream-token", got.header.Get("Authorization"))

		assert.Equal(t, "application/json", got.header.Get("Content-Type"), "Content-Type must not be overridable")
		assert.Equal(t, string(models.ExportSprints), got.header.Get("X-TaskFlow-Event"))
		assert.NotEqual(t, "0", got.header.Get("X-TaskFlow-Timestamp"))
		assert.NotEqual(t, "sha256=deadbeef", got.header.Get("X-TaskFlow-Signature"))
		assert.Equal(t, exExpectSignature(secret, got.header.Get("X-TaskFlow-Timestamp"), got.body),
			got.header.Get("X-TaskFlow-Signature"), "the real signature must survive the override attempt")
	})
}

// ---------------------------------------------------------------------------
// Push: delivery accounting
// ---------------------------------------------------------------------------

func TestExportService_Push_DeliveryAccounting(t *testing.T) {
	t.Run("a 2xx records a successful delivery", func(t *testing.T) {
		cases := []struct {
			name   string
			status int
		}{
			{name: "200", status: http.StatusOK},
			{name: "201", status: http.StatusCreated},
			{name: "204", status: http.StatusNoContent},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				endpoint, receiver := exStartReceiver(t, tc.status)

				cfg := exConfig()
				cfg.MaxRetries = 2
				f := exNewFixture(cfg)

				var recorded *models.ExportDelivery
				f.repo.On("RecordDelivery", mock.Anything, mock.AnythingOfType("*models.ExportDelivery")).
					Run(func(args mock.Arguments) { recorded = args.Get(1).(*models.ExportDelivery) }).
					Return(nil).Once()

				delivery, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
					URL:   endpoint,
					Kind:  models.ExportFull,
					Query: exQuery(),
				})

				require.NoError(t, err)
				require.NotNil(t, delivery)
				assert.Equal(t, 1, receiver.count(), "a success must not be retried")

				f.repo.AssertExpectations(t)
				require.NotNil(t, recorded, "RecordDelivery must be called on the success path")
				assert.Same(t, delivery, recorded)
				assert.Equal(t, "SUCCESS", recorded.Status)
				assert.Equal(t, 1, recorded.Attempts)
				require.NotNil(t, recorded.StatusCode)
				assert.Equal(t, tc.status, *recorded.StatusCode)
				assert.Nil(t, recorded.Error)
				assert.Nil(t, recorded.TargetID, "an ad-hoc push has no target")
				assert.Equal(t, endpoint, recorded.URL)
				assert.Equal(t, models.ExportFull, recorded.Kind)
				assert.Positive(t, recorded.PayloadSize)

				f.repo.AssertNotCalled(t, "TouchTarget",
					mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			})
		}
	})

	t.Run("a 5xx is retried and records a failed delivery", func(t *testing.T) {
		endpoint, receiver := exStartReceiver(t, http.StatusInternalServerError)

		cfg := exConfig()
		cfg.MaxRetries = 1 // 1 retry => 2 attempts
		f := exNewFixture(cfg)

		var recorded *models.ExportDelivery
		f.repo.On("RecordDelivery", mock.Anything, mock.AnythingOfType("*models.ExportDelivery")).
			Run(func(args mock.Arguments) { recorded = args.Get(1).(*models.ExportDelivery) }).
			Return(nil).Once()

		delivery, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
			URL:   endpoint,
			Kind:  models.ExportAnalytics,
			Query: exQuery(),
		})

		// A failed push must hand back BOTH the error and the audit record.
		require.Error(t, err)
		require.NotNil(t, delivery, "a failed delivery is still reported to the caller")
		assert.ErrorIs(t, err, apperrors.ErrExportFailed)

		assert.Equal(t, 2, receiver.count(), "MaxRetries=1 means two attempts")

		f.repo.AssertExpectations(t)
		require.NotNil(t, recorded, "RecordDelivery must be called on the failure path too")
		assert.Same(t, delivery, recorded)
		assert.Equal(t, "FAILED", recorded.Status)
		assert.Equal(t, 2, recorded.Attempts)
		require.NotNil(t, recorded.StatusCode)
		assert.Equal(t, http.StatusInternalServerError, *recorded.StatusCode)
		require.NotNil(t, recorded.Error)
		assert.Contains(t, *recorded.Error, "500")
	})

	t.Run("a 4xx is not retried but is still recorded as failed", func(t *testing.T) {
		endpoint, receiver := exStartReceiver(t, http.StatusBadRequest)

		cfg := exConfig()
		cfg.MaxRetries = 3
		f := exNewFixture(cfg)

		var recorded *models.ExportDelivery
		f.repo.On("RecordDelivery", mock.Anything, mock.AnythingOfType("*models.ExportDelivery")).
			Run(func(args mock.Arguments) { recorded = args.Get(1).(*models.ExportDelivery) }).
			Return(nil).Once()

		delivery, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
			URL:   endpoint,
			Kind:  models.ExportTasks,
			Query: exQuery(),
		})

		require.Error(t, err)
		require.NotNil(t, delivery)
		assert.ErrorIs(t, err, apperrors.ErrExportFailed)
		assert.Equal(t, 1, receiver.count(), "a client error must not be retried")

		require.NotNil(t, recorded)
		assert.Equal(t, "FAILED", recorded.Status)
		assert.Equal(t, 1, recorded.Attempts)
		f.repo.AssertExpectations(t)
	})

	t.Run("a retry that eventually succeeds records SUCCESS with the attempt count", func(t *testing.T) {
		endpoint, receiver := exStartReceiver(t, http.StatusServiceUnavailable, http.StatusOK)

		cfg := exConfig()
		cfg.MaxRetries = 2
		f := exNewFixture(cfg)

		var recorded *models.ExportDelivery
		f.repo.On("RecordDelivery", mock.Anything, mock.AnythingOfType("*models.ExportDelivery")).
			Run(func(args mock.Arguments) { recorded = args.Get(1).(*models.ExportDelivery) }).
			Return(nil).Once()

		delivery, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
			URL:   endpoint,
			Kind:  models.ExportTasks,
			Query: exQuery(),
		})

		require.NoError(t, err)
		require.NotNil(t, delivery)
		assert.Equal(t, 2, receiver.count())

		require.NotNil(t, recorded)
		assert.Equal(t, "SUCCESS", recorded.Status)
		assert.Equal(t, 2, recorded.Attempts)
		f.repo.AssertExpectations(t)
	})

	t.Run("an oversized payload is rejected before any HTTP request", func(t *testing.T) {
		endpoint, receiver := exStartReceiver(t, http.StatusOK)

		cfg := exConfig()
		cfg.MaxPayloadSize = 64 // far smaller than any real envelope
		f := exNewFixture(cfg)

		delivery, err := f.svc.Push(context.Background(), exUser(), service.PushRequest{
			URL:   endpoint,
			Kind:  models.ExportFull,
			Query: exQuery(),
		})

		require.Error(t, err)
		assert.Nil(t, delivery)
		assert.ErrorIs(t, err, apperrors.ErrExportFailed)
		assert.Contains(t, err.Error(), "64 byte limit")

		assert.Equal(t, 0, receiver.count(), "nothing may be sent when the payload is too large")
		f.repo.AssertNotCalled(t, "RecordDelivery", mock.Anything, mock.Anything)
	})

	t.Run("target and url are mutually exclusive", func(t *testing.T) {
		endpoint, receiver := exStartReceiver(t, http.StatusOK)
		targetID := uuid.New()

		cases := []struct {
			name string
			req  service.PushRequest
		}{
			{name: "neither", req: service.PushRequest{Kind: models.ExportFull, Query: exQuery()}},
			{name: "both", req: service.PushRequest{TargetID: &targetID, URL: endpoint, Kind: models.ExportFull, Query: exQuery()}},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f := exNewFixture(exConfig())

				delivery, err := f.svc.Push(context.Background(), exUser(), tc.req)

				require.Error(t, err)
				assert.Nil(t, delivery)
				assert.ErrorIs(t, err, apperrors.ErrInvalidExportURL)
				assert.Equal(t, 0, receiver.count())
			})
		}
	})

	t.Run("a nil user is unauthorized", func(t *testing.T) {
		f := exNewFixture(exConfig())

		delivery, err := f.svc.Push(context.Background(), nil, service.PushRequest{
			URL: "https://8.8.8.8/hook", Kind: models.ExportFull, Query: exQuery(),
		})

		require.Error(t, err)
		assert.Nil(t, delivery)
		assert.ErrorIs(t, err, apperrors.ErrUnauthorized)
	})
}

// ---------------------------------------------------------------------------
// BuildEnvelope
// ---------------------------------------------------------------------------

func TestExportService_BuildEnvelope(t *testing.T) {
	user := exUser()
	summary := exSummary()

	t.Run("each kind carries only the sections it calls for", func(t *testing.T) {
		cases := []struct {
			name         string
			kind         models.ExportKind
			wantAnalytic bool
			wantTasks    bool
			wantSprints  bool
			wantSummary  bool
		}{
			{name: "ANALYTICS_SNAPSHOT", kind: models.ExportAnalytics, wantAnalytic: true},
			{name: "TASKS", kind: models.ExportTasks, wantTasks: true},
			{name: "SPRINTS", kind: models.ExportSprints, wantSprints: true},
			{name: "AI_SUMMARY", kind: models.ExportAISummary, wantSummary: true},
			{name: "FULL", kind: models.ExportFull, wantAnalytic: true, wantTasks: true, wantSprints: true, wantSummary: true},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f := exNewFixture(exConfig())

				envelope, err := f.svc.BuildEnvelope(context.Background(), user, tc.kind, exQuery(), summary)

				require.NoError(t, err)
				require.NotNil(t, envelope)

				assert.Equal(t, models.ExportSchema, envelope.Schema)
				assert.Equal(t, tc.kind, envelope.Kind)
				assert.NotEmpty(t, envelope.Nonce, "every envelope must carry a replay nonce")
				assert.Len(t, envelope.Nonce, 32, "nonce is 16 random bytes hex-encoded")
				assert.False(t, envelope.GeneratedAt.IsZero())
				assert.Equal(t, user.ID, envelope.User.ID)
				assert.Equal(t, user.Email, envelope.User.Email)

				// Range and Counts always describe the whole window.
				require.NotNil(t, envelope.Range)
				assert.Equal(t, exDashboard().Range, *envelope.Range)
				assert.Equal(t, map[string]int{"tasks": 3, "sprints": 1, "open": 2, "done": 1}, envelope.Counts)

				if tc.wantAnalytic {
					assert.NotNil(t, envelope.Analytics)
				} else {
					assert.Nil(t, envelope.Analytics, "%s must not carry analytics", tc.kind)
				}
				if tc.wantTasks {
					assert.Len(t, envelope.Tasks, 3)
				} else {
					assert.Nil(t, envelope.Tasks, "%s must not carry tasks", tc.kind)
				}
				if tc.wantSprints {
					assert.Len(t, envelope.Sprints, 1)
				} else {
					assert.Nil(t, envelope.Sprints, "%s must not carry sprints", tc.kind)
				}
				if tc.wantSummary {
					assert.Same(t, summary, envelope.Summary)
				} else {
					assert.Nil(t, envelope.Summary, "%s must not carry the AI summary", tc.kind)
				}

				// The task window is scoped to the analytics query.
				require.NotNil(t, f.tasks.lastFilter.From)
				require.NotNil(t, f.tasks.lastFilter.To)
				assert.Equal(t, exQuery().From, *f.tasks.lastFilter.From)
				assert.Equal(t, exQuery().To, *f.tasks.lastFilter.To)
			})
		}
	})

	t.Run("an unknown kind is rejected", func(t *testing.T) {
		cases := []struct {
			name string
			kind models.ExportKind
		}{
			{name: "empty", kind: models.ExportKind("")},
			{name: "unknown word", kind: models.ExportKind("EVERYTHING")},
			{name: "wrong case", kind: models.ExportKind("full")},
			{name: "near miss", kind: models.ExportKind("ANALYTICS")},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				f := exNewFixture(exConfig())

				envelope, err := f.svc.BuildEnvelope(context.Background(), user, tc.kind, exQuery(), summary)

				require.Error(t, err)
				assert.Nil(t, envelope)
				assert.ErrorIs(t, err, apperrors.ErrInvalidExportKind)
				assert.Zero(t, f.analytics.calls, "an invalid kind must be rejected before any data is loaded")
				assert.Zero(t, f.tasks.calls)
				assert.Zero(t, f.sprints.calls)
			})
		}
	})

	t.Run("a nil user is unauthorized", func(t *testing.T) {
		f := exNewFixture(exConfig())

		envelope, err := f.svc.BuildEnvelope(context.Background(), nil, models.ExportFull, exQuery(), summary)

		require.Error(t, err)
		assert.Nil(t, envelope)
		assert.ErrorIs(t, err, apperrors.ErrUnauthorized)
		assert.Zero(t, f.analytics.calls)
	})

	t.Run("the nonce is fresh on every build", func(t *testing.T) {
		f := exNewFixture(exConfig())

		seen := make(map[string]struct{}, 8)
		for range 8 {
			envelope, err := f.svc.BuildEnvelope(context.Background(), user, models.ExportFull, exQuery(), summary)
			require.NoError(t, err)
			_, dup := seen[envelope.Nonce]
			assert.False(t, dup, "nonce %q was reused", envelope.Nonce)
			seen[envelope.Nonce] = struct{}{}
		}
	})

	t.Run("an AI_SUMMARY envelope without a summary stays nil", func(t *testing.T) {
		f := exNewFixture(exConfig())

		envelope, err := f.svc.BuildEnvelope(context.Background(), user, models.ExportAISummary, exQuery(), nil)

		require.NoError(t, err)
		require.NotNil(t, envelope)
		assert.Nil(t, envelope.Summary)
	})

	t.Run("counts reflect the loaded tasks", func(t *testing.T) {
		f := exNewFixture(exConfig())
		f.tasks.tasks = []models.Task{
			{ID: uuid.New(), Title: "a", Status: models.StatusDone},
			{ID: uuid.New(), Title: "b", Status: models.StatusDone},
			{ID: uuid.New(), Title: "c", Status: models.StatusTodo},
			{ID: uuid.New(), Title: "d", Status: models.StatusInReview},
		}
		f.sprints.sprints = nil

		envelope, err := f.svc.BuildEnvelope(context.Background(), user, models.ExportFull, exQuery(), summary)

		require.NoError(t, err)
		assert.Equal(t, map[string]int{"tasks": 4, "sprints": 0, "open": 2, "done": 2}, envelope.Counts)
	})

	t.Run("a downstream failure is propagated", func(t *testing.T) {
		sentinel := apperrors.ErrTaskNotFound

		t.Run("analytics", func(t *testing.T) {
			f := exNewFixture(exConfig())
			f.analytics.err = sentinel

			envelope, err := f.svc.BuildEnvelope(context.Background(), user, models.ExportFull, exQuery(), summary)

			require.Error(t, err)
			assert.Nil(t, envelope)
			assert.ErrorIs(t, err, sentinel)
		})

		t.Run("tasks", func(t *testing.T) {
			f := exNewFixture(exConfig())
			f.tasks.err = sentinel

			envelope, err := f.svc.BuildEnvelope(context.Background(), user, models.ExportFull, exQuery(), summary)

			require.Error(t, err)
			assert.Nil(t, envelope)
			assert.ErrorIs(t, err, sentinel)
		})

		t.Run("sprints", func(t *testing.T) {
			f := exNewFixture(exConfig())
			f.sprints.err = sentinel

			envelope, err := f.svc.BuildEnvelope(context.Background(), user, models.ExportFull, exQuery(), summary)

			require.Error(t, err)
			assert.Nil(t, envelope)
			assert.ErrorIs(t, err, sentinel)
		})
	})
}
