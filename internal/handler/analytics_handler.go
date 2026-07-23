package handler

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/gin-gonic/gin"
)

type AnalyticsHandler struct {
	analytics service.Analytics
}

func NewAnalyticsHandler(analytics service.Analytics) *AnalyticsHandler {
	return &AnalyticsHandler{analytics: analytics}
}

func (h *AnalyticsHandler) Dashboard(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	q, ok := parseAnalyticsQuery(c, h.analytics)
	if !ok {
		return
	}

	dashboard, err := h.analytics.Dashboard(c.Request.Context(), userID, q)
	if respondServiceError(c, err, "Failed to build analytics dashboard") {
		return
	}

	c.JSON(http.StatusOK, dashboard)
}

func (h *AnalyticsHandler) Export(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	if format == "" {
		format = "json"
	}
	if format != "json" && format != "csv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format must be one of: json, csv"})
		return
	}

	q, ok := parseAnalyticsQuery(c, h.analytics)
	if !ok {
		return
	}

	dashboard, err := h.analytics.Dashboard(c.Request.Context(), userID, q)
	if respondServiceError(c, err, "Failed to build analytics export") {
		return
	}

	c.Header("Content-Disposition", analyticsAttachment(q, dashboard.GeneratedAt, format))

	if format == "json" {
		body, err := json.MarshalIndent(dashboard, "", "  ")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode analytics export"})
			return
		}
		c.Data(http.StatusOK, "application/json", body)
		return
	}

	c.Header("Content-Type", "text/csv")
	c.Status(http.StatusOK)
	if err := writeAnalyticsCSV(c.Writer, dashboard); err != nil {
		// The body is already on the wire, so the failure can only be reported
		// through the request's error list.
		_ = c.Error(fmt.Errorf("write analytics csv: %w", err))
	}
}

func analyticsAttachment(q models.AnalyticsQuery, generatedAt time.Time, format string) string {
	loc := q.Location
	if loc == nil {
		loc = time.UTC
	}
	day := generatedAt.In(loc).Format("20060102")
	return fmt.Sprintf(`attachment; filename="taskflow-analytics-%s-%s.%s"`, q.Period, day, format)
}

func writeAnalyticsCSV(w io.Writer, d *models.Dashboard) error {
	cw := csv.NewWriter(w)
	row := func(cells ...string) { _ = cw.Write(cells) }
	gap := func() { _ = cw.Write(nil) }

	t := d.Totals
	bestDay := ""
	if t.BestDay != nil {
		bestDay = *t.BestDay
	}

	row("TOTALS")
	row("metric", "value")
	row("total_tasks", strconv.Itoa(t.TotalTasks))
	row("created_in_range", strconv.Itoa(t.CreatedInRange))
	row("completed_in_range", strconv.Itoa(t.CompletedInRange))
	row("open_now", strconv.Itoa(t.OpenNow))
	row("todo", strconv.Itoa(t.Todo))
	row("in_progress", strconv.Itoa(t.InProgress))
	row("in_review", strconv.Itoa(t.InReview))
	row("done", strconv.Itoa(t.Done))
	row("overdue", strconv.Itoa(t.Overdue))
	row("due_soon", strconv.Itoa(t.DueSoon))
	row("blocked", strconv.Itoa(t.Blocked))
	row("completion_rate", analyticsFloat(t.CompletionRate))
	row("avg_cycle_time_hours", analyticsFloat(t.AvgCycleTimeHours))
	row("median_cycle_time_hours", analyticsFloat(t.MedianCycleTimeHours))
	row("planned_hours", analyticsFloat(t.PlannedHours))
	row("buffer_hours", analyticsFloat(t.BufferHours))
	row("spent_hours", analyticsFloat(t.SpentHours))
	row("estimate_accuracy", analyticsFloat(t.EstimateAccuracy))
	row("avg_completed_per_day", analyticsFloat(t.AvgCompletedPerDay))
	row("active_days", strconv.Itoa(t.ActiveDays))
	row("current_streak_days", strconv.Itoa(t.CurrentStreakDays))
	row("longest_streak_days", strconv.Itoa(t.LongestStreakDays))
	row("best_day", bestDay)
	row("best_day_count", strconv.Itoa(t.BestDayCount))
	gap()

	row("SERIES")
	row("bucket", "label", "created", "completed", "open_at_end", "completed_hours")
	for _, b := range d.Series {
		row(
			b.BucketStart.Format(time.RFC3339),
			b.Label,
			strconv.Itoa(b.Created),
			strconv.Itoa(b.Completed),
			strconv.Itoa(b.OpenAtEnd),
			analyticsFloat(b.CompletedHours),
		)
	}
	gap()

	row("STATUS")
	row("status", "count", "percent")
	for _, s := range d.StatusBreakdown {
		row(string(s.Status), strconv.Itoa(s.Count), analyticsFloat(s.Percent))
	}
	gap()

	row("PRIORITY")
	row("priority", "total", "done", "open", "overdue", "completion_rate", "estimate_hours")
	for _, p := range d.PriorityBreakdown {
		row(
			string(p.Priority),
			strconv.Itoa(p.Total),
			strconv.Itoa(p.Done),
			strconv.Itoa(p.Open),
			strconv.Itoa(p.Overdue),
			analyticsFloat(p.CompletionRate),
			analyticsFloat(p.EstimateHours),
		)
	}
	gap()

	row("CYCLE_TIME")
	row("bucket", "avg_hours", "median_hours", "p90_hours", "samples")
	for _, p := range d.CycleTimeSeries {
		row(
			p.BucketStart.Format(time.RFC3339),
			analyticsFloat(p.AvgHours),
			analyticsFloat(p.MedianHours),
			analyticsFloat(p.P90Hours),
			strconv.Itoa(p.Samples),
		)
	}
	gap()

	row("HEATMAP")
	row("date", "count")
	for _, day := range d.Heatmap {
		row(day.Date, strconv.Itoa(day.Count))
	}
	gap()

	cw.Flush()
	return cw.Error()
}

func analyticsFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
