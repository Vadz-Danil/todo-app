package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	analyticsDayLayout     = "2006-01-02"
	analyticsDueSoonWindow = 72 * time.Hour
	analyticsMaxRangeYears = 5
	analyticsMaxReviewers  = 8
	analyticsMaxAgingItems = 10
)

var analyticsWeekdayLabels = [7]string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// analyticsBucket is one slot of the time series. start/end are what the API
// emits; from is start clamped to the query window so bucket counts always sum
// back to the window totals.
type analyticsBucket struct {
	start time.Time
	from  time.Time
	end   time.Time
	label string
}

type AnalyticsService struct {
	repo   repository.AnalyticsRepository
	logger *zap.Logger
}

func NewAnalyticsService(repo repository.AnalyticsRepository, logger *zap.Logger) *AnalyticsService {
	return &AnalyticsService{
		repo:   repo,
		logger: logger,
	}
}

func (s *AnalyticsService) ResolveQuery(period, from, to, granularity, tz string) (models.AnalyticsQuery, error) {
	zone := strings.TrimSpace(tz)
	if zone == "" {
		zone = "UTC"
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return models.AnalyticsQuery{}, fmt.Errorf("%w: unknown timezone %q", apperrors.ErrInvalidDateRange, zone)
	}

	p := models.Period(strings.ToLower(strings.TrimSpace(period)))
	if p == "" {
		p = models.PeriodWeek
	}
	if !p.IsValid() {
		return models.AnalyticsQuery{}, apperrors.ErrInvalidPeriod
	}

	now := time.Now().In(loc)
	start, end := time.Time{}, s.endOfDay(now, loc)

	switch p {
	case models.PeriodToday:
		start = s.startOfDay(now, loc)
	case models.PeriodWeek:
		start = s.startOfDay(now.AddDate(0, 0, -6), loc)
	case models.PeriodMonth:
		start = s.startOfDay(now.AddDate(0, 0, -29), loc)
	case models.PeriodQuarter:
		start = s.startOfDay(now.AddDate(0, 0, -89), loc)
	case models.PeriodHalfYear:
		start = s.startOfDay(now.AddDate(0, 0, -179), loc)
	case models.PeriodYear:
		start = s.startOfDay(now.AddDate(0, 0, -364), loc)
	case models.PeriodCustom:
		start, end, err = s.parseCustomRange(from, to, loc)
		if err != nil {
			return models.AnalyticsQuery{}, err
		}
	}

	g := models.Granularity(strings.ToLower(strings.TrimSpace(granularity)))
	if g == "" {
		g = s.deriveGranularity(start, end, loc)
	} else if !g.IsValid() {
		return models.AnalyticsQuery{}, apperrors.ErrInvalidGranularity
	}

	return models.AnalyticsQuery{
		Period:      p,
		From:        start,
		To:          end,
		Granularity: g,
		Location:    loc,
	}, nil
}

func (s *AnalyticsService) Dashboard(ctx context.Context, userID uuid.UUID, q models.AnalyticsQuery) (*models.Dashboard, error) {
	loc := q.Location
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)

	if q.From.IsZero() {
		first, err := s.repo.FirstTaskAt(ctx, userID)
		if err != nil {
			s.logger.Error("failed to load first task timestamp", zap.Error(err), zap.String("user_id", userID.String()))
			return nil, fmt.Errorf("failed to resolve all-time window: %w", err)
		}
		if first != nil {
			q.From = s.startOfDay(first.In(loc), loc)
		} else {
			q.From = s.startOfDay(now.AddDate(0, 0, -365), loc)
		}
	}
	if !q.Granularity.IsValid() {
		q.Granularity = s.deriveGranularity(q.From, q.To, loc)
	}

	tasks, err := s.repo.LoadWindow(ctx, userID, q.From, q.To)
	if err != nil {
		s.logger.Error("failed to load analytics window", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to load analytics window: %w", err)
	}

	totalTasks, statusCounts, err := s.repo.GlobalCounts(ctx, userID)
	if err != nil {
		s.logger.Error("failed to load global task counts", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to load global task counts: %w", err)
	}

	allCompletionDays, err := s.repo.CompletionDays(ctx, userID, loc.String())
	if err != nil {
		s.logger.Error("failed to load completion days", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to load completion days: %w", err)
	}

	sprints, err := s.repo.SprintSummaries(ctx, userID, q.From, q.To)
	if err != nil {
		s.logger.Error("failed to load sprint summaries", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to load sprint summaries: %w", err)
	}

	span := q.To.Sub(q.From)
	prevFrom, prevTo := q.From.Add(-span), q.From.Add(-time.Nanosecond)
	prevTasks, err := s.repo.LoadWindow(ctx, userID, prevFrom, prevTo)
	if err != nil {
		s.logger.Error("failed to load previous analytics window", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to load previous analytics window: %w", err)
	}

	days := s.windowDays(q.From, q.To, loc)
	dueSoonLimit := now.Add(analyticsDueSoonWindow)

	createdInRange, completedInRange := 0, 0
	overdue, dueSoon, blocked := 0, 0, 0
	plannedHours, bufferHours, spentHours := 0.0, 0.0, 0.0
	accountedSpent, accountedEstimate := 0.0, 0.0

	weekdayCreated, weekdayCompleted, weekdayOccurrences := [7]int{}, [7]int{}, [7]int{}
	completedPerDay := make(map[string]int, len(tasks))

	for _, t := range tasks {
		if s.inWindow(t.CreatedAt, q.From, q.To) {
			createdInRange++
			plannedHours += s.hours(t.EstimateHours)
			bufferHours += s.hours(t.BufferHours)
			weekdayCreated[s.weekdayIndex(t.CreatedAt.In(loc))]++
		}

		if t.CompletedAt != nil && s.inWindow(*t.CompletedAt, q.From, q.To) {
			completedInRange++
			spentHours += s.hours(t.SpentHours)
			weekdayCompleted[s.weekdayIndex(t.CompletedAt.In(loc))]++
			completedPerDay[s.dayKey(t.CompletedAt.In(loc))]++
			if t.EstimateHours != nil && t.SpentHours != nil {
				accountedEstimate += *t.EstimateHours
				accountedSpent += *t.SpentHours
			}
		}

		if t.Status == models.StatusDone {
			continue
		}
		if t.DueDate != nil {
			if t.DueDate.Before(now) {
				overdue++
			} else if !t.DueDate.After(dueSoonLimit) {
				dueSoon++
			}
		}
		if t.Blockers != nil && strings.TrimSpace(*t.Blockers) != "" {
			blocked++
		}
	}

	heatmap := make([]models.DayCount, 0, days)
	activeDays, bestDayCount := 0, 0
	var bestDay *string
	for d := s.startOfDay(q.From, loc); !d.After(q.To); d = d.AddDate(0, 0, 1) {
		key := s.dayKey(d)
		count := completedPerDay[key]
		heatmap = append(heatmap, models.DayCount{Date: key, Count: count})
		weekdayOccurrences[s.weekdayIndex(d)]++
		if count > 0 {
			activeDays++
		}
		if count > bestDayCount {
			bestDayCount = count
			day := key
			bestDay = &day
		}
	}

	cycles := s.cycleSamples(tasks, q.From, q.To)
	sort.Float64s(cycles)
	openAtEnd := s.openAt(tasks, q.To)
	rate := s.completionRate(completedInRange, openAtEnd)
	currentStreak, longestStreak := s.streaks(allCompletionDays, now)

	totals := models.Totals{
		TotalTasks:           totalTasks,
		CreatedInRange:       createdInRange,
		CompletedInRange:     completedInRange,
		OpenNow:              totalTasks - statusCounts[models.StatusDone],
		Todo:                 statusCounts[models.StatusTodo],
		InProgress:           statusCounts[models.StatusInProgress],
		InReview:             statusCounts[models.StatusInReview],
		Done:                 statusCounts[models.StatusDone],
		Overdue:              overdue,
		DueSoon:              dueSoon,
		Blocked:              blocked,
		CompletionRate:       s.round(rate, 4),
		AvgCycleTimeHours:    s.round(s.mean(cycles), 2),
		MedianCycleTimeHours: s.round(s.median(cycles), 2),
		PlannedHours:         s.round(plannedHours, 2),
		BufferHours:          s.round(bufferHours, 2),
		SpentHours:           s.round(spentHours, 2),
		EstimateAccuracy:     s.round(s.ratio(accountedSpent, accountedEstimate), 4),
		AvgCompletedPerDay:   s.round(float64(completedInRange)/float64(days), 2),
		ActiveDays:           activeDays,
		CurrentStreakDays:    currentStreak,
		LongestStreakDays:    longestStreak,
		BestDay:              bestDay,
		BestDayCount:         bestDayCount,
	}

	prevCreated := s.countCreated(prevTasks, prevFrom, prevTo)
	prevCycles := s.cycleSamples(prevTasks, prevFrom, prevTo)
	prevCompleted := len(prevCycles)
	comparison := models.Comparison{
		PreviousFrom:   prevFrom,
		PreviousTo:     prevTo,
		Created:        s.delta(float64(createdInRange), float64(prevCreated), 0),
		Completed:      s.delta(float64(completedInRange), float64(prevCompleted), 0),
		CompletionRate: s.delta(rate, s.completionRate(prevCompleted, s.openAt(prevTasks, prevTo)), 4),
		CycleTime:      s.delta(s.mean(cycles), s.mean(prevCycles), 2),
	}

	buckets := s.buildBuckets(q.From, q.To, q.Granularity, loc)
	series := make([]models.TimeBucket, 0, len(buckets))
	cycleSeries := make([]models.CycleTimePoint, 0, len(buckets))
	for _, b := range buckets {
		bucketCreated, bucketCompleted, bucketHours := 0, 0, 0.0
		samples := make([]float64, 0, len(tasks))
		for _, t := range tasks {
			if s.inWindow(t.CreatedAt, b.from, b.end) {
				bucketCreated++
			}
			if t.CompletedAt != nil && s.inWindow(*t.CompletedAt, b.from, b.end) {
				bucketCompleted++
				bucketHours += s.hours(t.SpentHours)
				samples = append(samples, s.cycleHours(t))
			}
		}
		sort.Float64s(samples)

		series = append(series, models.TimeBucket{
			BucketStart:    b.start,
			BucketEnd:      b.end,
			Label:          b.label,
			Created:        bucketCreated,
			Completed:      bucketCompleted,
			OpenAtEnd:      s.openAt(tasks, b.end),
			CompletedHours: s.round(bucketHours, 2),
		})
		cycleSeries = append(cycleSeries, models.CycleTimePoint{
			BucketStart: b.start,
			Label:       b.label,
			AvgHours:    s.round(s.mean(samples), 2),
			MedianHours: s.round(s.median(samples), 2),
			P90Hours:    s.round(s.percentile(samples, 0.9), 2),
			Samples:     len(samples),
		})
	}

	statusBreakdown := make([]models.StatusBucket, 0, len(models.BoardStatuses))
	for _, st := range models.BoardStatuses {
		count := statusCounts[st]
		statusBreakdown = append(statusBreakdown, models.StatusBucket{
			Status:  st,
			Count:   count,
			Percent: s.round(s.ratio(float64(count), float64(totalTasks)), 4),
		})
	}

	priorities := append([]models.TaskPriority(nil), models.Priorities...)
	sort.Slice(priorities, func(i, j int) bool { return priorities[i].Rank() < priorities[j].Rank() })

	priorityBreakdown := make([]models.PriorityBucket, 0, len(priorities))
	for _, p := range priorities {
		bucket := models.PriorityBucket{Priority: p}
		samples := make([]float64, 0, len(tasks))
		for _, t := range tasks {
			if t.Priority != p {
				continue
			}
			bucket.Total++
			bucket.EstimateHours += s.hours(t.EstimateHours)
			if t.Status == models.StatusDone {
				bucket.Done++
			} else {
				bucket.Open++
				if t.DueDate != nil && t.DueDate.Before(now) {
					bucket.Overdue++
				}
			}
			if t.CompletedAt != nil && s.inWindow(*t.CompletedAt, q.From, q.To) {
				samples = append(samples, s.cycleHours(t))
			}
		}
		bucket.Percent = s.round(s.ratio(float64(bucket.Total), float64(len(tasks))), 4)
		bucket.CompletionRate = s.round(s.ratio(float64(bucket.Done), float64(bucket.Total)), 4)
		bucket.EstimateHours = s.round(bucket.EstimateHours, 2)
		bucket.AvgCycleHours = s.round(s.mean(samples), 2)
		priorityBreakdown = append(priorityBreakdown, bucket)
	}

	weekdayLoad := make([]models.WeekdayBucket, 0, len(analyticsWeekdayLabels))
	for i, label := range analyticsWeekdayLabels {
		weekdayLoad = append(weekdayLoad, models.WeekdayBucket{
			Weekday:   i + 1,
			Label:     label,
			Completed: weekdayCompleted[i],
			Created:   weekdayCreated[i],
			AvgPerDay: s.round(s.ratio(float64(weekdayCompleted[i]), float64(weekdayOccurrences[i])), 2),
		})
	}

	dashboard := &models.Dashboard{
		Range: models.RangeInfo{
			Period:      q.Period,
			From:        q.From,
			To:          q.To,
			Granularity: q.Granularity,
			Timezone:    loc.String(),
			Label:       s.rangeLabel(q, loc),
			Days:        days,
		},
		Totals:            totals,
		Comparison:        comparison,
		Series:            series,
		StatusBreakdown:   statusBreakdown,
		PriorityBreakdown: priorityBreakdown,
		CycleTimeSeries:   cycleSeries,
		Heatmap:           heatmap,
		WeekdayLoad:       weekdayLoad,
		Reviewers:         s.reviewerBuckets(tasks),
		SprintProgress:    sprints,
		AgingWIP:          s.agingWIP(tasks, now),
		GeneratedAt:       time.Now().UTC(),
	}
	if dashboard.SprintProgress == nil {
		dashboard.SprintProgress = make([]models.SprintStatsItem, 0)
	}

	return dashboard, nil
}

func (s *AnalyticsService) parseCustomRange(from, to string, loc *time.Location) (time.Time, time.Time, error) {
	from, to = strings.TrimSpace(from), strings.TrimSpace(to)
	if from == "" || to == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from and to are required when period is custom", apperrors.ErrInvalidDateRange)
	}

	start, err := s.parseBoundary(from, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := s.parseBoundary(to, loc)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	start, end = s.startOfDay(start, loc), s.endOfDay(end, loc)
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: to must not be earlier than from", apperrors.ErrInvalidDateRange)
	}
	if end.After(start.AddDate(analyticsMaxRangeYears, 0, 0)) {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: range must not exceed %d years", apperrors.ErrInvalidDateRange, analyticsMaxRangeYears)
	}

	return start, end, nil
}

func (s *AnalyticsService) parseBoundary(raw string, loc *time.Location) (time.Time, error) {
	if t, err := time.ParseInLocation(time.RFC3339, raw, loc); err == nil {
		return t.In(loc), nil
	}
	if t, err := time.ParseInLocation(analyticsDayLayout, raw, loc); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%w: %q must be YYYY-MM-DD or RFC3339", apperrors.ErrInvalidDateRange, raw)
}

func (s *AnalyticsService) deriveGranularity(from, to time.Time, loc *time.Location) models.Granularity {
	switch days := s.windowDays(from, to, loc); {
	case days <= 45:
		return models.GranularityDay
	case days <= 400:
		return models.GranularityWeek
	default:
		return models.GranularityMonth
	}
}

func (s *AnalyticsService) rangeLabel(q models.AnalyticsQuery, loc *time.Location) string {
	switch q.Period {
	case models.PeriodToday:
		return "Today"
	case models.PeriodWeek:
		return "Last 7 days"
	case models.PeriodMonth:
		return "Last 30 days"
	case models.PeriodQuarter:
		return "Last 90 days"
	case models.PeriodHalfYear:
		return "Last 6 months"
	case models.PeriodYear:
		return "Last 12 months"
	case models.PeriodAllTime:
		return "All time"
	default:
		return q.From.In(loc).Format("2 Jan") + " - " + q.To.In(loc).Format("2 Jan 2006")
	}
}

func (s *AnalyticsService) buildBuckets(from, to time.Time, g models.Granularity, loc *time.Location) []analyticsBucket {
	buckets := make([]analyticsBucket, 0)
	if to.Before(from) {
		return buckets
	}

	for cur := s.bucketStart(from, g, loc); !cur.After(to); {
		next := s.bucketNext(cur, g)
		end := next.Add(-time.Nanosecond)
		if end.After(to) {
			end = to
		}
		start := cur
		if start.Before(from) {
			start = from
		}

		buckets = append(buckets, analyticsBucket{
			start: cur,
			from:  start,
			end:   end,
			label: s.bucketLabel(cur, g),
		})
		cur = next
	}

	return buckets
}

func (s *AnalyticsService) bucketStart(t time.Time, g models.Granularity, loc *time.Location) time.Time {
	switch g {
	case models.GranularityWeek:
		day := s.startOfDay(t, loc)
		return day.AddDate(0, 0, -s.weekdayIndex(day))
	case models.GranularityMonth:
		y, m, _ := t.In(loc).Date()
		return time.Date(y, m, 1, 0, 0, 0, 0, loc)
	default:
		return s.startOfDay(t, loc)
	}
}

func (s *AnalyticsService) bucketNext(start time.Time, g models.Granularity) time.Time {
	switch g {
	case models.GranularityWeek:
		return start.AddDate(0, 0, 7)
	case models.GranularityMonth:
		return start.AddDate(0, 1, 0)
	default:
		return start.AddDate(0, 0, 1)
	}
}

func (s *AnalyticsService) bucketLabel(start time.Time, g models.Granularity) string {
	if g == models.GranularityMonth {
		return start.Format("Jan 2006")
	}
	return start.Format("2 Jan")
}

func (s *AnalyticsService) reviewerBuckets(tasks []models.Task) []models.ReviewerBucket {
	grouped := make(map[string]models.ReviewerBucket)
	for _, t := range tasks {
		if t.Reviewer == nil {
			continue
		}
		name := strings.TrimSpace(*t.Reviewer)
		if name == "" {
			continue
		}

		bucket := grouped[name]
		bucket.Reviewer = name
		bucket.Total++
		switch t.Status {
		case models.StatusInReview:
			bucket.InReview++
		case models.StatusDone:
			bucket.Completed++
		}
		grouped[name] = bucket
	}

	reviewers := make([]models.ReviewerBucket, 0, len(grouped))
	for _, bucket := range grouped {
		reviewers = append(reviewers, bucket)
	}
	sort.Slice(reviewers, func(i, j int) bool {
		if reviewers[i].Total != reviewers[j].Total {
			return reviewers[i].Total > reviewers[j].Total
		}
		return reviewers[i].Reviewer < reviewers[j].Reviewer
	})
	if len(reviewers) > analyticsMaxReviewers {
		reviewers = reviewers[:analyticsMaxReviewers]
	}

	return reviewers
}

func (s *AnalyticsService) agingWIP(tasks []models.Task, now time.Time) []models.AgingItem {
	items := make([]models.AgingItem, 0)
	for _, t := range tasks {
		if t.Status != models.StatusInProgress && t.Status != models.StatusInReview {
			continue
		}

		start := t.CreatedAt
		if t.StartedAt != nil {
			start = *t.StartedAt
		}
		age := now.Sub(start).Hours()
		if age < 0 {
			age = 0
		}

		items = append(items, models.AgingItem{
			TaskID:    t.ID.String(),
			Title:     t.Title,
			Status:    t.Status,
			Priority:  t.Priority,
			AgeHours:  s.round(age, 1),
			Reviewer:  t.Reviewer,
			IsOverdue: t.DueDate != nil && t.DueDate.Before(now),
		})
	}

	sort.SliceStable(items, func(i, j int) bool { return items[i].AgeHours > items[j].AgeHours })
	if len(items) > analyticsMaxAgingItems {
		items = items[:analyticsMaxAgingItems]
	}

	return items
}

// streaks walks all-time completion days; the current streak only counts when
// it reaches today or yesterday.
func (s *AnalyticsService) streaks(days []models.DayCount, now time.Time) (current, longest int) {
	parsed := make([]time.Time, 0, len(days))
	for _, d := range days {
		if d.Count <= 0 {
			continue
		}
		day, err := time.Parse(analyticsDayLayout, d.Date)
		if err != nil {
			continue
		}
		parsed = append(parsed, day)
	}
	if len(parsed) == 0 {
		return 0, 0
	}
	sort.Slice(parsed, func(i, j int) bool { return parsed[i].Before(parsed[j]) })

	run := 0
	var prev time.Time
	for i, day := range parsed {
		switch {
		case i == 0:
			run = 1
		case day.Equal(prev):
			continue
		case day.Sub(prev) == 24*time.Hour:
			run++
		default:
			run = 1
		}
		if run > longest {
			longest = run
		}
		prev = day
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if prev.Equal(today) || prev.Equal(today.AddDate(0, 0, -1)) {
		current = run
	}

	return current, longest
}

func (s *AnalyticsService) countCreated(tasks []models.Task, from, to time.Time) int {
	count := 0
	for _, t := range tasks {
		if s.inWindow(t.CreatedAt, from, to) {
			count++
		}
	}
	return count
}

func (s *AnalyticsService) cycleSamples(tasks []models.Task, from, to time.Time) []float64 {
	samples := make([]float64, 0, len(tasks))
	for _, t := range tasks {
		if t.CompletedAt != nil && s.inWindow(*t.CompletedAt, from, to) {
			samples = append(samples, s.cycleHours(t))
		}
	}
	return samples
}

func (s *AnalyticsService) cycleHours(t models.Task) float64 {
	if t.CompletedAt == nil {
		return 0
	}
	start := t.CreatedAt
	if t.StartedAt != nil {
		start = *t.StartedAt
	}
	if hours := t.CompletedAt.Sub(start).Hours(); hours > 0 {
		return hours
	}
	return 0
}

func (s *AnalyticsService) openAt(tasks []models.Task, at time.Time) int {
	count := 0
	for _, t := range tasks {
		if t.CreatedAt.After(at) {
			continue
		}
		if t.CompletedAt == nil || t.CompletedAt.After(at) {
			count++
		}
	}
	return count
}

func (s *AnalyticsService) completionRate(completed, openAtEnd int) float64 {
	rate := s.ratio(float64(completed), float64(completed+openAtEnd))
	if rate > 1 {
		return 1
	}
	return rate
}

func (s *AnalyticsService) delta(current, previous float64, decimals int) models.Delta {
	d := models.Delta{
		Current:  s.round(current, decimals),
		Previous: s.round(previous, decimals),
	}
	if previous != 0 {
		pct := s.round((current-previous)/previous, 4)
		d.ChangePct = &pct
	}
	return d
}

func (s *AnalyticsService) mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

// median expects sorted values.
func (s *AnalyticsService) median(sorted []float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

// percentile expects sorted values and uses the nearest-rank method.
func (s *AnalyticsService) percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return sorted[idx]
}

func (s *AnalyticsService) ratio(numerator, denominator float64) float64 {
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}

func (s *AnalyticsService) round(v float64, decimals int) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	factor := math.Pow(10, float64(decimals))
	return math.Round(v*factor) / factor
}

func (s *AnalyticsService) hours(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func (s *AnalyticsService) inWindow(t, from, to time.Time) bool {
	return !t.Before(from) && !t.After(to)
}

func (s *AnalyticsService) startOfDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

func (s *AnalyticsService) endOfDay(t time.Time, loc *time.Location) time.Time {
	return s.startOfDay(t, loc).AddDate(0, 0, 1).Add(-time.Nanosecond)
}

func (s *AnalyticsService) dayKey(t time.Time) string {
	return t.Format(analyticsDayLayout)
}

// weekdayIndex is Monday-first: Mon=0 ... Sun=6.
func (s *AnalyticsService) weekdayIndex(t time.Time) int {
	return (int(t.Weekday()) + 6) % 7
}

func (s *AnalyticsService) windowDays(from, to time.Time, loc *time.Location) int {
	if to.Before(from) {
		return 1
	}
	start, end := s.startOfDay(from, loc), s.startOfDay(to, loc)
	fy, fm, fd := start.Date()
	ty, tm, td := end.Date()
	elapsed := time.Date(ty, tm, td, 0, 0, 0, 0, time.UTC).Sub(time.Date(fy, fm, fd, 0, 0, 0, 0, time.UTC))

	days := int(elapsed/(24*time.Hour)) + 1
	if days < 1 {
		return 1
	}
	return days
}
