package models

import "time"

// Period is a named, self-describing analytics window.
type Period string

const (
	PeriodToday     Period = "today"
	PeriodWeek      Period = "week"
	PeriodMonth     Period = "month"
	PeriodQuarter   Period = "quarter"
	PeriodHalfYear  Period = "half_year"
	PeriodYear      Period = "year"
	PeriodAllTime   Period = "all_time"
	PeriodCustom    Period = "custom"
)

func (p Period) IsValid() bool {
	switch p {
	case PeriodToday, PeriodWeek, PeriodMonth, PeriodQuarter, PeriodHalfYear, PeriodYear, PeriodAllTime, PeriodCustom:
		return true
	}
	return false
}

// Granularity is the bucket width of the time series.
type Granularity string

const (
	GranularityDay   Granularity = "day"
	GranularityWeek  Granularity = "week"
	GranularityMonth Granularity = "month"
)

func (g Granularity) IsValid() bool {
	switch g {
	case GranularityDay, GranularityWeek, GranularityMonth:
		return true
	}
	return false
}

// AnalyticsQuery is the resolved, timezone-aware window an analytics read runs over.
type AnalyticsQuery struct {
	Period      Period
	From        time.Time
	To          time.Time
	Granularity Granularity
	Location    *time.Location
}

type RangeInfo struct {
	Period      Period      `json:"period"`
	From        time.Time   `json:"from"`
	To          time.Time   `json:"to"`
	Granularity Granularity `json:"granularity"`
	Timezone    string      `json:"timezone"`
	Label       string      `json:"label"`
	Days        int         `json:"days"`
}

// Dashboard is the full analytics payload backing the charts screen.
type Dashboard struct {
	Range             RangeInfo         `json:"range"`
	Totals            Totals            `json:"totals"`
	Comparison        Comparison        `json:"comparison"`
	Series            []TimeBucket      `json:"series"`
	StatusBreakdown   []StatusBucket    `json:"status_breakdown"`
	PriorityBreakdown []PriorityBucket  `json:"priority_breakdown"`
	CycleTimeSeries   []CycleTimePoint  `json:"cycle_time_series"`
	Heatmap           []DayCount        `json:"heatmap"`
	WeekdayLoad       []WeekdayBucket   `json:"weekday_load"`
	Reviewers         []ReviewerBucket  `json:"reviewers"`
	SprintProgress    []SprintStatsItem `json:"sprint_progress"`
	AgingWIP          []AgingItem       `json:"aging_wip"`
	GeneratedAt       time.Time         `json:"generated_at"`
}

type Totals struct {
	TotalTasks           int     `json:"total_tasks"`
	CreatedInRange       int     `json:"created_in_range"`
	CompletedInRange     int     `json:"completed_in_range"`
	OpenNow              int     `json:"open_now"`
	Todo                 int     `json:"todo"`
	InProgress           int     `json:"in_progress"`
	InReview             int     `json:"in_review"`
	Done                 int     `json:"done"`
	Overdue              int     `json:"overdue"`
	DueSoon              int     `json:"due_soon"`
	Blocked              int     `json:"blocked"`
	CompletionRate       float64 `json:"completion_rate"`
	AvgCycleTimeHours    float64 `json:"avg_cycle_time_hours"`
	MedianCycleTimeHours float64 `json:"median_cycle_time_hours"`
	PlannedHours         float64 `json:"planned_hours"`
	BufferHours          float64 `json:"buffer_hours"`
	SpentHours           float64 `json:"spent_hours"`
	EstimateAccuracy     float64 `json:"estimate_accuracy"`
	AvgCompletedPerDay   float64 `json:"avg_completed_per_day"`
	ActiveDays           int     `json:"active_days"`
	CurrentStreakDays    int     `json:"current_streak_days"`
	LongestStreakDays    int     `json:"longest_streak_days"`
	BestDay              *string `json:"best_day,omitempty"`
	BestDayCount         int     `json:"best_day_count"`
}

// Delta compares a metric against the immediately preceding window of equal length.
type Delta struct {
	Current   float64  `json:"current"`
	Previous  float64  `json:"previous"`
	ChangePct *float64 `json:"change_pct,omitempty"`
}

type Comparison struct {
	PreviousFrom   time.Time `json:"previous_from"`
	PreviousTo     time.Time `json:"previous_to"`
	Created        Delta     `json:"created"`
	Completed      Delta     `json:"completed"`
	CompletionRate Delta     `json:"completion_rate"`
	CycleTime      Delta     `json:"cycle_time"`
}

type TimeBucket struct {
	BucketStart    time.Time `json:"bucket_start"`
	BucketEnd      time.Time `json:"bucket_end"`
	Label          string    `json:"label"`
	Created        int       `json:"created"`
	Completed      int       `json:"completed"`
	OpenAtEnd      int       `json:"open_at_end"`
	CompletedHours float64   `json:"completed_hours"`
}

type StatusBucket struct {
	Status  TaskStatus `json:"status"`
	Count   int        `json:"count"`
	Percent float64    `json:"percent"`
}

type PriorityBucket struct {
	Priority       TaskPriority `json:"priority"`
	Total          int          `json:"total"`
	Done           int          `json:"done"`
	Open           int          `json:"open"`
	Overdue        int          `json:"overdue"`
	Percent        float64      `json:"percent"`
	CompletionRate float64      `json:"completion_rate"`
	EstimateHours  float64      `json:"estimate_hours"`
	AvgCycleHours  float64      `json:"avg_cycle_hours"`
}

type CycleTimePoint struct {
	BucketStart time.Time `json:"bucket_start"`
	Label       string    `json:"label"`
	AvgHours    float64   `json:"avg_hours"`
	MedianHours float64   `json:"median_hours"`
	P90Hours    float64   `json:"p90_hours"`
	Samples     int       `json:"samples"`
}

type DayCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type WeekdayBucket struct {
	Weekday   int     `json:"weekday"`
	Label     string  `json:"label"`
	Completed int     `json:"completed"`
	Created   int     `json:"created"`
	AvgPerDay float64 `json:"avg_per_day"`
}

type ReviewerBucket struct {
	Reviewer  string `json:"reviewer"`
	InReview  int    `json:"in_review"`
	Completed int    `json:"completed"`
	Total     int    `json:"total"`
}

type SprintStatsItem struct {
	SprintID       string       `json:"sprint_id"`
	Name           string       `json:"name"`
	Status         SprintStatus `json:"status"`
	StartsOn       time.Time    `json:"starts_on"`
	EndsOn         time.Time    `json:"ends_on"`
	TotalTasks     int          `json:"total_tasks"`
	DoneTasks      int          `json:"done_tasks"`
	PlannedHours   float64      `json:"planned_hours"`
	CompletionRate float64      `json:"completion_rate"`
}

// AgingItem surfaces work-in-progress that has been sitting in a column too long.
type AgingItem struct {
	TaskID    string       `json:"task_id"`
	Title     string       `json:"title"`
	Status    TaskStatus   `json:"status"`
	Priority  TaskPriority `json:"priority"`
	AgeHours  float64      `json:"age_hours"`
	Reviewer  *string      `json:"reviewer,omitempty"`
	IsOverdue bool         `json:"is_overdue"`
}
