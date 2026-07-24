package service_test

import (
	"context"
	"testing"
	"time"

	// Embed the IANA database so the timezone assertions below do not depend on
	// the host having /usr/share/zoneinfo. Standard library, no new dependency.
	_ "time/tzdata"

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
// mock repository
// ---------------------------------------------------------------------------

type anMockRepo struct {
	mock.Mock
}

func (m *anMockRepo) LoadWindow(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]models.Task, error) {
	args := m.Called(ctx, userID, from, to)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.Task), args.Error(1)
}

func (m *anMockRepo) GlobalCounts(ctx context.Context, userID uuid.UUID) (int, map[models.TaskStatus]int, error) {
	args := m.Called(ctx, userID)
	var counts map[models.TaskStatus]int
	if args.Get(1) != nil {
		counts = args.Get(1).(map[models.TaskStatus]int)
	}
	return args.Int(0), counts, args.Error(2)
}

func (m *anMockRepo) CompletionDays(ctx context.Context, userID uuid.UUID, loc *time.Location) ([]models.DayCount, error) {
	args := m.Called(ctx, userID, loc)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.DayCount), args.Error(1)
}

func (m *anMockRepo) StatusChanges(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]models.StatusChange, error) {
	args := m.Called(ctx, userID, from, to)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.StatusChange), args.Error(1)
}

func (m *anMockRepo) SprintSummaries(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]models.SprintStatsItem, error) {
	args := m.Called(ctx, userID, from, to)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]models.SprintStatsItem), args.Error(1)
}

func (m *anMockRepo) FirstTaskAt(ctx context.Context, userID uuid.UUID) (*time.Time, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*time.Time), args.Error(1)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

const anStamp = "2006-01-02 15:04"

var anUserID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

// Stable IDs so ordering assertions can name the task they expect.
var (
	anIDT1 = uuid.MustParse("00000000-0000-0000-0000-000000000001")
	anIDT2 = uuid.MustParse("00000000-0000-0000-0000-000000000002")
	anIDT3 = uuid.MustParse("00000000-0000-0000-0000-000000000003")
	anIDT4 = uuid.MustParse("00000000-0000-0000-0000-000000000004")
	anIDT5 = uuid.MustParse("00000000-0000-0000-0000-000000000005")
	anIDT6 = uuid.MustParse("00000000-0000-0000-0000-000000000006")
	anIDT7 = uuid.MustParse("00000000-0000-0000-0000-000000000007")
	anIDT8 = uuid.MustParse("00000000-0000-0000-0000-000000000008")
	anIDT9 = uuid.MustParse("00000000-0000-0000-0000-000000000009")
)

func anPtr[T any](v T) *T { return &v }

// anAt parses "2006-01-02 15:04" as UTC.
func anAt(value string) time.Time {
	t, err := time.ParseInLocation(anStamp, value, time.UTC)
	if err != nil {
		panic(err)
	}
	return t
}

func anService(repo *anMockRepo) *service.AnalyticsService {
	return service.NewAnalyticsService(repo, zap.NewNop())
}

// anWindow is the hand-built analytics window used by the Dashboard tests:
// Mon 2026-06-01 00:00:00 UTC .. Sun 2026-06-07 23:59:59.999999999 UTC.
// Exactly 7 days, exactly one Monday-to-Sunday week.
func anWindow(g models.Granularity) models.AnalyticsQuery {
	from := anAt("2026-06-01 00:00")
	return models.AnalyticsQuery{
		Period:      models.PeriodCustom,
		From:        from,
		To:          from.AddDate(0, 0, 7).Add(-time.Nanosecond),
		Granularity: g,
		Location:    time.UTC,
	}
}

// anTasks is the fixture LoadWindow returns for anWindow. Nine tasks, chosen so
// every aggregate can be recomputed by hand:
//
//	id  status       prio     created      started      completed    cycle
//	T1  DONE         HIGH     06-01 09:00  06-01 10:00  06-01 12:00   2h  (uses StartedAt)
//	T2  DONE         URGENT   06-02 08:00  -            06-03 08:00  24h  (falls back to CreatedAt)
//	T3  DONE         MEDIUM   06-03 09:00  06-03 12:00  06-03 10:00   0h  (negative -> floored)
//	T4  DONE         HIGH     05-30 06:00  06-03 06:00  06-05 06:00  48h  (created before window)
//	T5  DONE         MEDIUM   06-06 10:00  06-06 12:00  06-06 22:00  10h
//	T6  TODO         URGENT   06-02 10:00  -            -             -   (overdue)
//	T7  IN_PROGRESS  MEDIUM   06-05 10:00  06-05 11:00  -             -   (due soon, blocked)
//	T8  IN_REVIEW    HIGH     06-07 10:00  -            -             -   (reviewer alice)
//	T9  TODO         MEDIUM   05-20 10:00  -            -             -   (created before window)
//
// No task has priority LOW, which proves empty priorities still show up.
// Due dates are relative to now so overdue/due-soon do not rot.
func anTasks(now time.Time) []models.Task {
	return []models.Task{
		{
			ID: anIDT1, UserID: anUserID, Title: "T1",
			Status: models.StatusDone, Priority: models.PriorityHigh,
			Reviewer:      anPtr("bob"),
			EstimateHours: anPtr(2.0), BufferHours: anPtr(0.5), SpentHours: anPtr(3.0),
			CreatedAt: anAt("2026-06-01 09:00"),
			StartedAt: anPtr(anAt("2026-06-01 10:00")), CompletedAt: anPtr(anAt("2026-06-01 12:00")),
		},
		{
			ID: anIDT2, UserID: anUserID, Title: "T2",
			Status: models.StatusDone, Priority: models.PriorityUrgent,
			EstimateHours: anPtr(4.0), BufferHours: anPtr(1.0), SpentHours: anPtr(5.0),
			CreatedAt:   anAt("2026-06-02 08:00"),
			CompletedAt: anPtr(anAt("2026-06-03 08:00")),
		},
		{
			ID: anIDT3, UserID: anUserID, Title: "T3",
			Status: models.StatusDone, Priority: models.PriorityMedium,
			SpentHours: anPtr(1.5),
			CreatedAt:  anAt("2026-06-03 09:00"),
			StartedAt:  anPtr(anAt("2026-06-03 12:00")), CompletedAt: anPtr(anAt("2026-06-03 10:00")),
		},
		{
			ID: anIDT4, UserID: anUserID, Title: "T4",
			Status: models.StatusDone, Priority: models.PriorityHigh,
			Reviewer:      anPtr("alice"),
			Blockers:      anPtr("was blocked, now done"),
			EstimateHours: anPtr(8.0), BufferHours: anPtr(2.0), SpentHours: anPtr(10.0),
			CreatedAt: anAt("2026-05-30 06:00"),
			StartedAt: anPtr(anAt("2026-06-03 06:00")), CompletedAt: anPtr(anAt("2026-06-05 06:00")),
		},
		{
			ID: anIDT5, UserID: anUserID, Title: "T5",
			Status: models.StatusDone, Priority: models.PriorityMedium,
			EstimateHours: anPtr(3.0),
			CreatedAt:     anAt("2026-06-06 10:00"),
			StartedAt:     anPtr(anAt("2026-06-06 12:00")), CompletedAt: anPtr(anAt("2026-06-06 22:00")),
		},
		{
			ID: anIDT6, UserID: anUserID, Title: "T6",
			Status: models.StatusTodo, Priority: models.PriorityUrgent,
			EstimateHours: anPtr(1.0), BufferHours: anPtr(0.5),
			DueDate:   anPtr(now.Add(-48 * time.Hour)),
			CreatedAt: anAt("2026-06-02 10:00"),
		},
		{
			ID: anIDT7, UserID: anUserID, Title: "T7",
			Status: models.StatusInProgress, Priority: models.PriorityMedium,
			Blockers:      anPtr("waiting on API"),
			EstimateHours: anPtr(6.0),
			DueDate:       anPtr(now.Add(24 * time.Hour)),
			CreatedAt:     anAt("2026-06-05 10:00"),
			StartedAt:     anPtr(anAt("2026-06-05 11:00")),
		},
		{
			ID: anIDT8, UserID: anUserID, Title: "T8",
			Status: models.StatusInReview, Priority: models.PriorityHigh,
			Reviewer:      anPtr("alice"),
			EstimateHours: anPtr(2.0), BufferHours: anPtr(0.5),
			CreatedAt: anAt("2026-06-07 10:00"),
		},
		{
			ID: anIDT9, UserID: anUserID, Title: "T9",
			Status: models.StatusTodo, Priority: models.PriorityMedium,
			EstimateHours: anPtr(5.0),
			DueDate:       anPtr(now.Add(200 * time.Hour)),
			CreatedAt:     anAt("2026-05-20 10:00"),
		},
	}
}

// anGlobalCounts is deliberately different from the 9-task window so the tests
// can prove StatusBreakdown reads the live snapshot, not the window.
func anGlobalCounts() map[models.TaskStatus]int {
	return map[models.TaskStatus]int{
		models.StatusTodo:       4,
		models.StatusInProgress: 2,
		models.StatusInReview:   1,
		models.StatusDone:       5,
	}
}

// anExpectWindows wires both LoadWindow calls: the query window and the
// immediately preceding window of equal length.
func anExpectWindows(repo *anMockRepo, q models.AnalyticsQuery, current, previous []models.Task) (prevFrom, prevTo time.Time) {
	span := q.To.Sub(q.From)
	prevFrom, prevTo = q.From.Add(-span), q.From.Add(-time.Nanosecond)
	repo.On("LoadWindow", mock.Anything, anUserID, q.From, q.To).Return(current, nil).Once()
	repo.On("LoadWindow", mock.Anything, anUserID, prevFrom, prevTo).Return(previous, nil).Once()
	return prevFrom, prevTo
}

// anStdRepo is the fixture wiring shared by most Dashboard subtests.
func anStdRepo(t *testing.T, q models.AnalyticsQuery, now time.Time) *anMockRepo {
	t.Helper()
	repo := new(anMockRepo)
	anExpectWindows(repo, q, anTasks(now), nil)
	repo.On("GlobalCounts", mock.Anything, anUserID).Return(12, anGlobalCounts(), nil)
	repo.On("CompletionDays", mock.Anything, anUserID, mock.Anything).Return(nil, nil)
	repo.On("SprintSummaries", mock.Anything, anUserID, q.From, q.To).Return(nil, nil)
	return repo
}

// anDayCounts turns "days ago" offsets into the all-time completion rows the
// streak calculation consumes.
func anDayCounts(now time.Time, count int, offsets ...int) []models.DayCount {
	out := make([]models.DayCount, 0, len(offsets))
	for _, off := range offsets {
		out = append(out, models.DayCount{
			Date:  now.AddDate(0, 0, -off).Format("2006-01-02"),
			Count: count,
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// ResolveQuery
// ---------------------------------------------------------------------------

func TestAnalyticsService_ResolveQuery_Defaults(t *testing.T) {
	svc := anService(new(anMockRepo))

	t.Run("empty period and tz default to week/UTC with day granularity", func(t *testing.T) {
		q, err := svc.ResolveQuery("", "", "", "", "")
		require.NoError(t, err)

		assert.Equal(t, models.PeriodWeek, q.Period)
		assert.Equal(t, models.GranularityDay, q.Granularity, "a 7 day span derives day granularity")
		require.NotNil(t, q.Location)
		assert.Equal(t, "UTC", q.Location.String())

		now := time.Now().UTC()
		startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		assert.Equal(t, startOfToday.AddDate(0, 0, -6), q.From, "week starts 6 days before today")
		assert.Equal(t, startOfToday.AddDate(0, 0, 1).Add(-time.Nanosecond), q.To)
	})

	t.Run("period is trimmed and lower-cased", func(t *testing.T) {
		q, err := svc.ResolveQuery("  MONTH  ", "", "", "", "")
		require.NoError(t, err)
		assert.Equal(t, models.PeriodMonth, q.Period)
	})

	t.Run("granularity is trimmed and lower-cased", func(t *testing.T) {
		q, err := svc.ResolveQuery("year", "", "", "  WEEK ", "")
		require.NoError(t, err)
		assert.Equal(t, models.GranularityWeek, q.Granularity)
	})

	t.Run("all_time leaves From zero and derives month granularity", func(t *testing.T) {
		q, err := svc.ResolveQuery("all_time", "", "", "", "")
		require.NoError(t, err)
		assert.Equal(t, models.PeriodAllTime, q.Period)
		assert.True(t, q.From.IsZero(), "all_time defers the lower bound to FirstTaskAt")
		assert.Equal(t, models.GranularityMonth, q.Granularity)
	})

	t.Run("today spans exactly one day", func(t *testing.T) {
		q, err := svc.ResolveQuery("today", "", "", "", "UTC")
		require.NoError(t, err)
		assert.Equal(t, models.GranularityDay, q.Granularity)
		assert.Equal(t, 24*time.Hour-time.Nanosecond, q.To.Sub(q.From))
	})
}

func TestAnalyticsService_ResolveQuery_WindowEndsAtEndOfToday(t *testing.T) {
	svc := anService(new(anMockRepo))

	// A real IANA zone: the window must close at 23:59:59.999999999 local, so
	// anything completed "right now" still falls inside it.
	kyiv, err := time.LoadLocation("Europe/Kyiv")
	require.NoError(t, err)

	periods := []models.Period{
		models.PeriodToday,
		models.PeriodWeek,
		models.PeriodMonth,
		models.PeriodQuarter,
		models.PeriodHalfYear,
		models.PeriodYear,
		models.PeriodAllTime,
	}

	for _, p := range periods {
		t.Run(string(p), func(t *testing.T) {
			q, err := svc.ResolveQuery(string(p), "", "", "", "Europe/Kyiv")
			require.NoError(t, err)

			require.NotNil(t, q.Location)
			assert.Equal(t, "Europe/Kyiv", q.Location.String())

			now := time.Now()
			local := now.In(kyiv)
			endOfToday := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, kyiv).
				AddDate(0, 0, 1).Add(-time.Nanosecond)

			assert.True(t, q.To.Equal(endOfToday),
				"window must end at end of today in Europe/Kyiv: got %s want %s", q.To, endOfToday)
			assert.True(t, q.To.After(now), "work done right now must still be inside the window")
			assert.False(t, q.From.After(now), "window must already have started")
		})
	}
}

func TestAnalyticsService_ResolveQuery_Granularity(t *testing.T) {
	svc := anService(new(anMockRepo))

	tests := []struct {
		name string
		from string
		to   string
		want models.Granularity
	}{
		// 2026-01-01 .. 2026-02-14 inclusive = 31 + 14 = 45 days.
		{name: "45 days is still day", from: "2026-01-01", to: "2026-02-14", want: models.GranularityDay},
		{name: "46 days rolls over to week", from: "2026-01-01", to: "2026-02-15", want: models.GranularityWeek},
		// 2024-01-01 .. 2025-02-03 inclusive = 400 days.
		{name: "400 days is still week", from: "2024-01-01", to: "2025-02-03", want: models.GranularityWeek},
		{name: "401 days rolls over to month", from: "2024-01-01", to: "2025-02-04", want: models.GranularityMonth},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := svc.ResolveQuery("custom", tc.from, tc.to, "", "UTC")
			require.NoError(t, err)
			assert.Equal(t, tc.want, q.Granularity)
		})
	}

	t.Run("explicit granularity wins over the derived one", func(t *testing.T) {
		q, err := svc.ResolveQuery("custom", "2026-01-01", "2026-01-07", "month", "UTC")
		require.NoError(t, err)
		assert.Equal(t, models.GranularityMonth, q.Granularity)
	})
}

func TestAnalyticsService_ResolveQuery_Custom(t *testing.T) {
	svc := anService(new(anMockRepo))

	t.Run("date-only bounds snap to start and end of day", func(t *testing.T) {
		q, err := svc.ResolveQuery("custom", "2026-03-10", "2026-03-12", "", "UTC")
		require.NoError(t, err)

		assert.Equal(t, models.PeriodCustom, q.Period)
		assert.Equal(t, anAt("2026-03-10 00:00"), q.From)
		assert.Equal(t, anAt("2026-03-13 00:00").Add(-time.Nanosecond), q.To)
	})

	t.Run("RFC3339 bounds snap to start and end of day", func(t *testing.T) {
		q, err := svc.ResolveQuery("custom", "2026-03-10T15:04:05Z", "2026-03-12T01:00:00Z", "", "UTC")
		require.NoError(t, err)

		assert.Equal(t, anAt("2026-03-10 00:00"), q.From, "From snaps back to the start of its day")
		assert.Equal(t, anAt("2026-03-13 00:00").Add(-time.Nanosecond), q.To, "To snaps forward to the end of its day")
	})

	t.Run("same day is a valid one-day range", func(t *testing.T) {
		q, err := svc.ResolveQuery("custom", "2026-03-10", "2026-03-10", "", "UTC")
		require.NoError(t, err)
		assert.Equal(t, 24*time.Hour-time.Nanosecond, q.To.Sub(q.From))
	})

	t.Run("exactly five years is accepted", func(t *testing.T) {
		_, err := svc.ResolveQuery("custom", "2015-01-01", "2019-12-31", "", "UTC")
		require.NoError(t, err)
	})
}

func TestAnalyticsService_ResolveQuery_Errors(t *testing.T) {
	svc := anService(new(anMockRepo))

	tests := []struct {
		name        string
		period      string
		from        string
		to          string
		granularity string
		tz          string
		wantErr     error
	}{
		{name: "unknown period", period: "fortnight", wantErr: apperrors.ErrInvalidPeriod},
		{name: "unknown granularity", period: "week", granularity: "hour", wantErr: apperrors.ErrInvalidGranularity},
		{name: "unknown timezone", period: "week", tz: "Mars/Phobos", wantErr: apperrors.ErrInvalidDateRange},
		{name: "custom without bounds", period: "custom", wantErr: apperrors.ErrInvalidDateRange},
		{name: "custom without to", period: "custom", from: "2026-01-01", wantErr: apperrors.ErrInvalidDateRange},
		{name: "custom with unparseable bound", period: "custom", from: "01/02/2026", to: "2026-01-05", wantErr: apperrors.ErrInvalidDateRange},
		{name: "custom with to before from", period: "custom", from: "2026-03-10", to: "2026-03-09", wantErr: apperrors.ErrInvalidDateRange},
		{name: "custom spanning more than five years", period: "custom", from: "2015-01-01", to: "2021-01-01", wantErr: apperrors.ErrInvalidDateRange},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := svc.ResolveQuery(tc.period, tc.from, tc.to, tc.granularity, tc.tz)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErr)
			assert.Equal(t, models.AnalyticsQuery{}, q, "a rejected query must come back zeroed")
		})
	}
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

func TestAnalyticsService_Dashboard_Totals(t *testing.T) {
	now := time.Now().UTC()
	q := anWindow(models.GranularityDay)
	repo := anStdRepo(t, q, now)

	dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
	require.NoError(t, err)
	require.NotNil(t, dash)

	tot := dash.Totals

	t.Run("range echoes the resolved query", func(t *testing.T) {
		assert.Equal(t, models.PeriodCustom, dash.Range.Period)
		assert.Equal(t, q.From, dash.Range.From)
		assert.Equal(t, q.To, dash.Range.To)
		assert.Equal(t, models.GranularityDay, dash.Range.Granularity)
		assert.Equal(t, "UTC", dash.Range.Timezone)
		assert.Equal(t, 7, dash.Range.Days)
		assert.Equal(t, "1 Jun - 7 Jun 2026", dash.Range.Label)
	})

	t.Run("counts", func(t *testing.T) {
		// T1,T2,T3,T5,T6,T7,T8 were created inside the window; T4 and T9 before it.
		assert.Equal(t, 7, tot.CreatedInRange)
		// T1..T5 completed inside the window.
		assert.Equal(t, 5, tot.CompletedInRange)
		// 6 completions land on 4 distinct days (06-01, 06-03 x2, 06-05, 06-06).
		assert.Equal(t, 4, tot.ActiveDays)
		require.NotNil(t, tot.BestDay)
		assert.Equal(t, "2026-06-03", *tot.BestDay)
		assert.Equal(t, 2, tot.BestDayCount)
		assert.InDelta(t, 5.0/7.0, tot.AvgCompletedPerDay, 0.005)
	})

	t.Run("status totals come from the live global snapshot", func(t *testing.T) {
		assert.Equal(t, 12, tot.TotalTasks, "12 globally even though the window holds 9")
		assert.Equal(t, 4, tot.Todo)
		assert.Equal(t, 2, tot.InProgress)
		assert.Equal(t, 1, tot.InReview)
		assert.Equal(t, 5, tot.Done)
		assert.Equal(t, 7, tot.OpenNow, "total 12 minus 5 done")
	})

	t.Run("due dates and blockers ignore completed work", func(t *testing.T) {
		assert.Equal(t, 1, tot.Overdue, "only T6")
		assert.Equal(t, 1, tot.DueSoon, "only T7, due inside the 72h window")
		assert.Equal(t, 1, tot.Blocked, "T7; T4 also has blockers but is DONE")
	})

	t.Run("hours", func(t *testing.T) {
		// Estimates of the 7 tasks created in range: 2+4+0+3+1+6+2.
		assert.InDelta(t, 18.0, tot.PlannedHours, 1e-9)
		// Buffers of the same 7: 0.5+1+0+0+0.5+0+0.5.
		assert.InDelta(t, 2.5, tot.BufferHours, 1e-9)
		// Spent on the 5 completed in range: 3+5+1.5+10+0.
		assert.InDelta(t, 19.5, tot.SpentHours, 1e-9)
		// Only T1, T2, T4 carry both estimate and spent: 18/14.
		assert.InDelta(t, 1.2857, tot.EstimateAccuracy, 1e-9)
	})

	t.Run("cycle time uses StartedAt with a CreatedAt fallback, floored at zero", func(t *testing.T) {
		// Samples: T1 2h (StartedAt), T2 24h (CreatedAt fallback), T3 0h
		// (completed before it started), T4 48h, T5 10h -> [0,2,10,24,48].
		assert.InDelta(t, 16.8, tot.AvgCycleTimeHours, 1e-9, "(0+2+10+24+48)/5")
		assert.InDelta(t, 10.0, tot.MedianCycleTimeHours, 1e-9, "middle of 5 sorted samples")
	})

	t.Run("completion rate is completed over completed plus open at end", func(t *testing.T) {
		// Open at 2026-06-07 23:59:59.999999999: T6, T7, T8, T9.
		assert.InDelta(t, 0.5556, tot.CompletionRate, 1e-9, "5/(5+4)")
		assert.LessOrEqual(t, tot.CompletionRate, 1.0)
	})

	repo.AssertExpectations(t)
}

func TestAnalyticsService_Dashboard_CompletionRateEdges(t *testing.T) {
	tests := []struct {
		name  string
		tasks []models.Task
		want  float64
	}{
		{
			name:  "zero denominator yields zero rather than NaN",
			tasks: nil,
			want:  0,
		},
		{
			name: "nothing completed yields zero",
			tasks: []models.Task{
				{ID: anIDT1, Status: models.StatusTodo, Priority: models.PriorityLow, CreatedAt: anAt("2026-06-02 09:00")},
				{ID: anIDT2, Status: models.StatusTodo, Priority: models.PriorityLow, CreatedAt: anAt("2026-06-03 09:00")},
			},
			want: 0,
		},
		{
			name: "everything completed is clamped to one",
			tasks: []models.Task{
				{
					ID: anIDT1, Status: models.StatusDone, Priority: models.PriorityLow,
					CreatedAt: anAt("2026-06-02 09:00"), CompletedAt: anPtr(anAt("2026-06-02 11:00")),
				},
				{
					ID: anIDT2, Status: models.StatusDone, Priority: models.PriorityLow,
					CreatedAt: anAt("2026-06-03 09:00"), CompletedAt: anPtr(anAt("2026-06-03 11:00")),
				},
			},
			want: 1,
		},
		{
			name: "half completed",
			tasks: []models.Task{
				{
					ID: anIDT1, Status: models.StatusDone, Priority: models.PriorityLow,
					CreatedAt: anAt("2026-06-02 09:00"), CompletedAt: anPtr(anAt("2026-06-02 11:00")),
				},
				{ID: anIDT2, Status: models.StatusTodo, Priority: models.PriorityLow, CreatedAt: anAt("2026-06-03 09:00")},
			},
			want: 0.5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := anWindow(models.GranularityDay)
			repo := new(anMockRepo)
			anExpectWindows(repo, q, tc.tasks, nil)
			repo.On("GlobalCounts", mock.Anything, anUserID).Return(0, map[models.TaskStatus]int{}, nil)
			repo.On("CompletionDays", mock.Anything, anUserID, time.UTC).Return(nil, nil)
			repo.On("SprintSummaries", mock.Anything, anUserID, q.From, q.To).Return(nil, nil)

			dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
			require.NoError(t, err)
			assert.InDelta(t, tc.want, dash.Totals.CompletionRate, 1e-9)
		})
	}
}

func TestAnalyticsService_Dashboard_Series(t *testing.T) {
	now := time.Now().UTC()
	q := anWindow(models.GranularityDay)
	repo := anStdRepo(t, q, now)

	dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
	require.NoError(t, err)

	// One bucket per day, gap-free, for the whole 7-day window.
	require.Len(t, dash.Series, 7, "a 7 day window at day granularity is exactly 7 buckets")
	require.Len(t, dash.CycleTimeSeries, 7)
	require.Len(t, dash.Heatmap, 7)

	want := []struct {
		day            string
		label          string
		created        int
		completed      int
		openAtEnd      int
		completedHours float64
	}{
		{day: "2026-06-01", label: "1 Jun", created: 1, completed: 1, openAtEnd: 2, completedHours: 3.0},
		{day: "2026-06-02", label: "2 Jun", created: 2, completed: 0, openAtEnd: 4, completedHours: 0},
		{day: "2026-06-03", label: "3 Jun", created: 1, completed: 2, openAtEnd: 3, completedHours: 6.5},
		{day: "2026-06-04", label: "4 Jun", created: 0, completed: 0, openAtEnd: 3, completedHours: 0},
		{day: "2026-06-05", label: "5 Jun", created: 1, completed: 1, openAtEnd: 3, completedHours: 10.0},
		{day: "2026-06-06", label: "6 Jun", created: 1, completed: 1, openAtEnd: 3, completedHours: 0},
		{day: "2026-06-07", label: "7 Jun", created: 1, completed: 0, openAtEnd: 4, completedHours: 0},
	}

	totalCreated, totalCompleted := 0, 0
	for i, w := range want {
		t.Run(w.day, func(t *testing.T) {
			got := dash.Series[i]
			start := anAt(w.day + " 00:00")

			assert.Equal(t, start, got.BucketStart)
			assert.Equal(t, start.AddDate(0, 0, 1).Add(-time.Nanosecond), got.BucketEnd)
			assert.Equal(t, w.label, got.Label)
			assert.Equal(t, w.created, got.Created)
			assert.Equal(t, w.completed, got.Completed)
			assert.Equal(t, w.openAtEnd, got.OpenAtEnd,
				"open = created at or before the bucket end and not completed by it")
			assert.InDelta(t, w.completedHours, got.CompletedHours, 1e-9)

			// The heatmap runs over the same gap-free day grid.
			assert.Equal(t, w.day, dash.Heatmap[i].Date)
			assert.Equal(t, w.completed, dash.Heatmap[i].Count)
		})
		totalCreated += w.created
		totalCompleted += w.completed
	}

	t.Run("buckets sum back to the window totals", func(t *testing.T) {
		assert.Equal(t, dash.Totals.CreatedInRange, totalCreated)
		assert.Equal(t, dash.Totals.CompletedInRange, totalCompleted)
	})

	t.Run("buckets are contiguous", func(t *testing.T) {
		for i := 1; i < len(dash.Series); i++ {
			assert.Equal(t, dash.Series[i-1].BucketEnd.Add(time.Nanosecond), dash.Series[i].BucketStart,
				"bucket %d must start where bucket %d ended", i, i-1)
		}
	})

	t.Run("empty buckets are present with zero counts", func(t *testing.T) {
		// 2026-06-04 has neither a creation nor a completion.
		empty := dash.Series[3]
		assert.Equal(t, anAt("2026-06-04 00:00"), empty.BucketStart)
		assert.Zero(t, empty.Created)
		assert.Zero(t, empty.Completed)
		assert.Zero(t, empty.CompletedHours)
		assert.Equal(t, 3, empty.OpenAtEnd, "open work carries across an empty bucket")

		assert.Zero(t, dash.CycleTimeSeries[3].Samples)
		assert.Zero(t, dash.CycleTimeSeries[3].AvgHours)
		assert.Zero(t, dash.CycleTimeSeries[3].MedianHours)
		assert.Zero(t, dash.CycleTimeSeries[3].P90Hours)
	})

	t.Run("per bucket cycle stats", func(t *testing.T) {
		// 2026-06-03 completed T2 (24h) and T3 (floored to 0h): sorted [0, 24].
		got := dash.CycleTimeSeries[2]
		assert.Equal(t, "3 Jun", got.Label)
		assert.Equal(t, 2, got.Samples)
		assert.InDelta(t, 12.0, got.AvgHours, 1e-9)
		assert.InDelta(t, 12.0, got.MedianHours, 1e-9, "even count averages the two middle samples")
		assert.InDelta(t, 24.0, got.P90Hours, 1e-9, "nearest rank: ceil(0.9*2)=2 -> the 2nd sample")
	})

	repo.AssertExpectations(t)
}

func TestAnalyticsService_Dashboard_CycleTimePercentiles(t *testing.T) {
	now := time.Now().UTC()
	// Month granularity collapses the whole window into a single bucket, so the
	// cycle point covers all five completions at once.
	q := anWindow(models.GranularityMonth)
	repo := anStdRepo(t, q, now)

	dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
	require.NoError(t, err)

	require.Len(t, dash.Series, 1)
	require.Len(t, dash.CycleTimeSeries, 1)

	point := dash.CycleTimeSeries[0]
	assert.Equal(t, anAt("2026-06-01 00:00"), point.BucketStart)
	assert.Equal(t, "Jun 2026", point.Label)

	// Sorted samples: [0, 2, 10, 24, 48].
	assert.Equal(t, 5, point.Samples)
	assert.InDelta(t, 16.8, point.AvgHours, 1e-9, "(0+2+10+24+48)/5")
	assert.InDelta(t, 10.0, point.MedianHours, 1e-9, "sorted[2]")
	assert.InDelta(t, 48.0, point.P90Hours, 1e-9, "nearest rank: ceil(0.9*5)=5 -> sorted[4]")

	repo.AssertExpectations(t)
}

func TestAnalyticsService_Dashboard_Breakdowns(t *testing.T) {
	now := time.Now().UTC()
	q := anWindow(models.GranularityDay)
	repo := anStdRepo(t, q, now)

	dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
	require.NoError(t, err)

	t.Run("priority breakdown is URGENT, HIGH, MEDIUM, LOW and keeps empty priorities", func(t *testing.T) {
		require.Len(t, dash.PriorityBreakdown, 4)

		order := make([]models.TaskPriority, 0, 4)
		for _, b := range dash.PriorityBreakdown {
			order = append(order, b.Priority)
		}
		assert.Equal(t, []models.TaskPriority{
			models.PriorityUrgent,
			models.PriorityHigh,
			models.PriorityMedium,
			models.PriorityLow,
		}, order)

		want := []struct {
			total, done, open, overdue int
			percent                    float64
			completionRate             float64
			estimateHours              float64
			avgCycleHours              float64
		}{
			// URGENT: T2 (done, 24h, est 4), T6 (todo, overdue, est 1).
			{total: 2, done: 1, open: 1, overdue: 1, percent: 2.0 / 9.0, completionRate: 0.5, estimateHours: 5, avgCycleHours: 24},
			// HIGH: T1 (done, 2h, est 2), T4 (done, 48h, est 8), T8 (in review, est 2).
			{total: 3, done: 2, open: 1, overdue: 0, percent: 3.0 / 9.0, completionRate: 2.0 / 3.0, estimateHours: 12, avgCycleHours: 25},
			// MEDIUM: T3 (done, 0h), T5 (done, 10h, est 3), T7 (est 6), T9 (est 5).
			{total: 4, done: 2, open: 2, overdue: 0, percent: 4.0 / 9.0, completionRate: 0.5, estimateHours: 14, avgCycleHours: 5},
			// LOW: no tasks at all, but the bucket is still emitted.
			{total: 0, done: 0, open: 0, overdue: 0, percent: 0, completionRate: 0, estimateHours: 0, avgCycleHours: 0},
		}

		for i, w := range want {
			got := dash.PriorityBreakdown[i]
			t.Run(string(got.Priority), func(t *testing.T) {
				assert.Equal(t, w.total, got.Total)
				assert.Equal(t, w.done, got.Done)
				assert.Equal(t, w.open, got.Open)
				assert.Equal(t, w.overdue, got.Overdue)
				assert.InDelta(t, w.percent, got.Percent, 0.0001)
				assert.InDelta(t, w.completionRate, got.CompletionRate, 0.0001)
				assert.InDelta(t, w.estimateHours, got.EstimateHours, 1e-9)
				assert.InDelta(t, w.avgCycleHours, got.AvgCycleHours, 1e-9)
			})
		}
	})

	t.Run("status breakdown follows BoardStatuses and uses the global snapshot", func(t *testing.T) {
		require.Len(t, dash.StatusBreakdown, len(models.BoardStatuses))

		wantCounts := anGlobalCounts()
		for i, st := range models.BoardStatuses {
			got := dash.StatusBreakdown[i]
			assert.Equal(t, st, got.Status, "column %d must be %s", i, st)
			assert.Equal(t, wantCounts[st], got.Count,
				"%s must come from GlobalCounts, not from the 9 windowed tasks", st)
			assert.InDelta(t, float64(wantCounts[st])/12.0, got.Percent, 0.0001)
		}
	})

	t.Run("weekday load has seven entries starting at Monday", func(t *testing.T) {
		require.Len(t, dash.WeekdayLoad, 7)

		// The window is Mon 2026-06-01 .. Sun 2026-06-07, so each weekday occurs once.
		want := []struct {
			weekday   int
			label     string
			created   int
			completed int
		}{
			{weekday: 1, label: "Mon", created: 1, completed: 1},
			{weekday: 2, label: "Tue", created: 2, completed: 0},
			{weekday: 3, label: "Wed", created: 1, completed: 2},
			{weekday: 4, label: "Thu", created: 0, completed: 0},
			{weekday: 5, label: "Fri", created: 1, completed: 1},
			{weekday: 6, label: "Sat", created: 1, completed: 1},
			{weekday: 7, label: "Sun", created: 1, completed: 0},
		}
		for i, w := range want {
			got := dash.WeekdayLoad[i]
			assert.Equal(t, w.weekday, got.Weekday)
			assert.Equal(t, w.label, got.Label)
			assert.Equal(t, w.created, got.Created)
			assert.Equal(t, w.completed, got.Completed)
			assert.InDelta(t, float64(w.completed), got.AvgPerDay, 1e-9, "each weekday occurs exactly once")
		}
	})

	t.Run("reviewers are ranked by volume", func(t *testing.T) {
		require.Len(t, dash.Reviewers, 2)
		assert.Equal(t, models.ReviewerBucket{Reviewer: "alice", InReview: 1, Completed: 1, Total: 2}, dash.Reviewers[0])
		assert.Equal(t, models.ReviewerBucket{Reviewer: "bob", InReview: 0, Completed: 1, Total: 1}, dash.Reviewers[1])
	})

	t.Run("aging WIP lists only in-flight work, oldest first", func(t *testing.T) {
		require.Len(t, dash.AgingWIP, 2, "only T7 (IN_PROGRESS) and T8 (IN_REVIEW)")

		assert.Equal(t, anIDT7.String(), dash.AgingWIP[0].TaskID)
		assert.Equal(t, models.StatusInProgress, dash.AgingWIP[0].Status)
		assert.Nil(t, dash.AgingWIP[0].Reviewer)

		assert.Equal(t, anIDT8.String(), dash.AgingWIP[1].TaskID)
		assert.Equal(t, models.StatusInReview, dash.AgingWIP[1].Status)
		require.NotNil(t, dash.AgingWIP[1].Reviewer)
		assert.Equal(t, "alice", *dash.AgingWIP[1].Reviewer)

		assert.Greater(t, dash.AgingWIP[0].AgeHours, dash.AgingWIP[1].AgeHours,
			"T7 started before T8 was created")
		assert.False(t, dash.AgingWIP[0].IsOverdue, "T7 is due in the future")
	})

	repo.AssertExpectations(t)
}

func TestAnalyticsService_Dashboard_Comparison(t *testing.T) {
	t.Run("empty previous window leaves change_pct nil instead of infinity", func(t *testing.T) {
		now := time.Now().UTC()
		q := anWindow(models.GranularityDay)
		repo := new(anMockRepo)
		prevFrom, prevTo := anExpectWindows(repo, q, anTasks(now), nil)
		repo.On("GlobalCounts", mock.Anything, anUserID).Return(12, anGlobalCounts(), nil)
		repo.On("CompletionDays", mock.Anything, anUserID, time.UTC).Return(nil, nil)
		repo.On("SprintSummaries", mock.Anything, anUserID, q.From, q.To).Return(nil, nil)

		dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
		require.NoError(t, err)

		cmp := dash.Comparison
		assert.Equal(t, prevFrom, cmp.PreviousFrom, "previous window is the same length, immediately before")
		assert.Equal(t, prevTo, cmp.PreviousTo)

		for name, d := range map[string]models.Delta{
			"created":         cmp.Created,
			"completed":       cmp.Completed,
			"completion_rate": cmp.CompletionRate,
			"cycle_time":      cmp.CycleTime,
		} {
			t.Run(name, func(t *testing.T) {
				assert.Zero(t, d.Previous)
				assert.Nil(t, d.ChangePct, "a zero baseline must not produce +Inf")
			})
		}

		assert.InDelta(t, 7.0, cmp.Created.Current, 1e-9)
		assert.InDelta(t, 5.0, cmp.Completed.Current, 1e-9)
		assert.InDelta(t, 16.8, cmp.CycleTime.Current, 1e-9)

		repo.AssertExpectations(t)
	})

	t.Run("non-empty previous window produces a signed change_pct", func(t *testing.T) {
		q := anWindow(models.GranularityDay)
		span := q.To.Sub(q.From)
		prevFrom := q.From.Add(-span)

		current := []models.Task{
			{ID: anIDT1, Status: models.StatusTodo, Priority: models.PriorityLow, CreatedAt: anAt("2026-06-03 09:00")},
		}
		previous := []models.Task{
			{ID: anIDT2, Status: models.StatusTodo, Priority: models.PriorityLow, CreatedAt: prevFrom.Add(24 * time.Hour)},
			{ID: anIDT3, Status: models.StatusTodo, Priority: models.PriorityLow, CreatedAt: prevFrom.Add(48 * time.Hour)},
		}

		repo := new(anMockRepo)
		anExpectWindows(repo, q, current, previous)
		repo.On("GlobalCounts", mock.Anything, anUserID).Return(3, map[models.TaskStatus]int{models.StatusTodo: 3}, nil)
		repo.On("CompletionDays", mock.Anything, anUserID, time.UTC).Return(nil, nil)
		repo.On("SprintSummaries", mock.Anything, anUserID, q.From, q.To).Return(nil, nil)

		dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
		require.NoError(t, err)

		created := dash.Comparison.Created
		assert.InDelta(t, 1.0, created.Current, 1e-9)
		assert.InDelta(t, 2.0, created.Previous, 1e-9)
		require.NotNil(t, created.ChangePct)
		assert.InDelta(t, -0.5, *created.ChangePct, 1e-9, "(1-2)/2")

		completed := dash.Comparison.Completed
		assert.Zero(t, completed.Current)
		assert.Zero(t, completed.Previous)
		assert.Nil(t, completed.ChangePct)

		repo.AssertExpectations(t)
	})
}

func TestAnalyticsService_Dashboard_Streaks(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name        string
		days        []models.DayCount
		wantCurrent int
		wantLongest int
	}{
		{
			name:        "no completions at all",
			days:        nil,
			wantCurrent: 0,
			wantLongest: 0,
		},
		{
			name:        "days with a zero count do not count",
			days:        anDayCounts(now, 0, 2, 1, 0),
			wantCurrent: 0,
			wantLongest: 0,
		},
		{
			name:        "run ending today keeps the current streak alive",
			days:        anDayCounts(now, 1, 4, 3, 2, 1, 0),
			wantCurrent: 5,
			wantLongest: 5,
		},
		{
			name:        "run ending yesterday keeps the current streak alive",
			days:        anDayCounts(now, 1, 2, 1),
			wantCurrent: 2,
			wantLongest: 2,
		},
		{
			name:        "run ending two days ago breaks the current streak",
			days:        anDayCounts(now, 1, 10, 9, 8, 7, 3, 2),
			wantCurrent: 0,
			wantLongest: 4,
		},
		{
			name:        "longest run comes from all time, not from the window",
			days:        anDayCounts(now, 1, 400, 399, 398, 397, 396, 395, 1, 0),
			wantCurrent: 2,
			wantLongest: 6,
		},
		{
			name:        "a single isolated day is a streak of one",
			days:        anDayCounts(now, 3, 0),
			wantCurrent: 1,
			wantLongest: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := anWindow(models.GranularityDay)
			repo := new(anMockRepo)
			anExpectWindows(repo, q, nil, nil)
			repo.On("GlobalCounts", mock.Anything, anUserID).Return(0, map[models.TaskStatus]int{}, nil)
			repo.On("CompletionDays", mock.Anything, anUserID, time.UTC).Return(tc.days, nil)
			repo.On("SprintSummaries", mock.Anything, anUserID, q.From, q.To).Return(nil, nil)

			dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
			require.NoError(t, err)

			assert.Equal(t, tc.wantCurrent, dash.Totals.CurrentStreakDays)
			assert.Equal(t, tc.wantLongest, dash.Totals.LongestStreakDays)
			repo.AssertExpectations(t)
		})
	}
}

func TestAnalyticsService_Dashboard_EmptyUser(t *testing.T) {
	// A brand new user: no tasks, no sprints, no completions, and an all-time
	// window whose lower bound has to be invented because FirstTaskAt is nil.
	q := models.AnalyticsQuery{
		Period:   models.PeriodAllTime,
		To:       time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 1).Add(-time.Nanosecond),
		Location: time.UTC,
	}

	repo := new(anMockRepo)
	repo.On("FirstTaskAt", mock.Anything, anUserID).Return(nil, nil).Once()
	repo.On("LoadWindow", mock.Anything, anUserID, mock.Anything, mock.Anything).Return(nil, nil)
	repo.On("GlobalCounts", mock.Anything, anUserID).Return(0, nil, nil)
	repo.On("CompletionDays", mock.Anything, anUserID, time.UTC).Return(nil, nil)
	repo.On("SprintSummaries", mock.Anything, anUserID, mock.Anything, mock.Anything).Return(nil, nil)

	dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
	require.NoError(t, err)
	require.NotNil(t, dash)

	t.Run("granularity is derived when the query left it unset", func(t *testing.T) {
		assert.True(t, dash.Range.Granularity.IsValid())
		assert.False(t, dash.Range.From.IsZero(), "all_time falls back to a synthetic lower bound")
	})

	t.Run("every slice is non-nil so the JSON never emits null", func(t *testing.T) {
		assert.NotNil(t, dash.Series)
		assert.NotNil(t, dash.StatusBreakdown)
		assert.NotNil(t, dash.PriorityBreakdown)
		assert.NotNil(t, dash.CycleTimeSeries)
		assert.NotNil(t, dash.Heatmap)
		assert.NotNil(t, dash.WeekdayLoad)
		assert.NotNil(t, dash.Reviewers)
		assert.NotNil(t, dash.SprintProgress)
		assert.NotNil(t, dash.AgingWIP)
	})

	t.Run("fixed-shape slices keep their shape", func(t *testing.T) {
		assert.Len(t, dash.StatusBreakdown, len(models.BoardStatuses))
		assert.Len(t, dash.PriorityBreakdown, len(models.Priorities))
		assert.Len(t, dash.WeekdayLoad, 7)
		assert.Len(t, dash.Series, len(dash.CycleTimeSeries))
		assert.NotEmpty(t, dash.Series, "even an empty user gets a gap-free bucket grid")
	})

	t.Run("variable-length slices are empty", func(t *testing.T) {
		assert.Empty(t, dash.Reviewers)
		assert.Empty(t, dash.SprintProgress)
		assert.Empty(t, dash.AgingWIP)
	})

	t.Run("totals are all zero without dividing by zero", func(t *testing.T) {
		assert.Equal(t, models.Totals{}, dash.Totals, "no tasks means an entirely zeroed Totals")
	})

	t.Run("breakdowns still enumerate every status and priority", func(t *testing.T) {
		for i, st := range models.BoardStatuses {
			assert.Equal(t, st, dash.StatusBreakdown[i].Status)
			assert.Zero(t, dash.StatusBreakdown[i].Count)
			assert.Zero(t, dash.StatusBreakdown[i].Percent)
		}
		for _, b := range dash.PriorityBreakdown {
			assert.Zero(t, b.Total)
			assert.Zero(t, b.Percent)
			assert.Zero(t, b.CompletionRate)
		}
	})

	t.Run("every series bucket is empty but present", func(t *testing.T) {
		for _, b := range dash.Series {
			assert.Zero(t, b.Created)
			assert.Zero(t, b.Completed)
			assert.Zero(t, b.OpenAtEnd)
		}
	})

	repo.AssertExpectations(t)
}

func TestAnalyticsService_Dashboard_SprintProgress(t *testing.T) {
	now := time.Now().UTC()
	q := anWindow(models.GranularityDay)

	sprints := []models.SprintStatsItem{
		{
			SprintID: "s-1", Name: "Sprint 1", Status: models.SprintActive,
			StartsOn: q.From, EndsOn: q.To, TotalTasks: 4, DoneTasks: 2,
			PlannedHours: 12, CompletionRate: 0.5,
		},
	}

	repo := new(anMockRepo)
	anExpectWindows(repo, q, anTasks(now), nil)
	repo.On("GlobalCounts", mock.Anything, anUserID).Return(12, anGlobalCounts(), nil)
	repo.On("CompletionDays", mock.Anything, anUserID, time.UTC).Return(nil, nil)
	repo.On("SprintSummaries", mock.Anything, anUserID, q.From, q.To).Return(sprints, nil)

	dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
	require.NoError(t, err)

	assert.Equal(t, sprints, dash.SprintProgress, "sprint summaries pass through untouched")
	repo.AssertExpectations(t)
}

func TestAnalyticsService_Dashboard_RepositoryErrors(t *testing.T) {
	failure := assert.AnError

	tests := []struct {
		name  string
		setup func(repo *anMockRepo, q models.AnalyticsQuery)
	}{
		{
			name: "LoadWindow fails",
			setup: func(repo *anMockRepo, q models.AnalyticsQuery) {
				repo.On("LoadWindow", mock.Anything, anUserID, q.From, q.To).Return(nil, failure).Once()
			},
		},
		{
			name: "GlobalCounts fails",
			setup: func(repo *anMockRepo, q models.AnalyticsQuery) {
				repo.On("LoadWindow", mock.Anything, anUserID, q.From, q.To).Return(nil, nil).Once()
				repo.On("GlobalCounts", mock.Anything, anUserID).Return(0, nil, failure).Once()
			},
		},
		{
			name: "CompletionDays fails",
			setup: func(repo *anMockRepo, q models.AnalyticsQuery) {
				repo.On("LoadWindow", mock.Anything, anUserID, q.From, q.To).Return(nil, nil).Once()
				repo.On("GlobalCounts", mock.Anything, anUserID).Return(0, nil, nil).Once()
				repo.On("CompletionDays", mock.Anything, anUserID, time.UTC).Return(nil, failure).Once()
			},
		},
		{
			name: "SprintSummaries fails",
			setup: func(repo *anMockRepo, q models.AnalyticsQuery) {
				repo.On("LoadWindow", mock.Anything, anUserID, q.From, q.To).Return(nil, nil).Once()
				repo.On("GlobalCounts", mock.Anything, anUserID).Return(0, nil, nil).Once()
				repo.On("CompletionDays", mock.Anything, anUserID, time.UTC).Return(nil, nil).Once()
				repo.On("SprintSummaries", mock.Anything, anUserID, q.From, q.To).Return(nil, failure).Once()
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := anWindow(models.GranularityDay)
			repo := new(anMockRepo)
			tc.setup(repo, q)

			dash, err := anService(repo).Dashboard(context.Background(), anUserID, q)
			require.Error(t, err)
			assert.ErrorIs(t, err, failure)
			assert.Nil(t, dash)
			repo.AssertExpectations(t)
		})
	}
}
