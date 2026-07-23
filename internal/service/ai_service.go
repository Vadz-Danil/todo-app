package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/repository"
	"todo-app/pkg/gemini"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	aiMaxPlanningItems = 40
	aiMaxHorizonWeeks  = 8
	aiMaxCapacityHours = 168.0
	aiDefaultCapacity  = 40.0
	aiMaxTaskHours     = 200.0
	aiDefaultEstimate  = 4.0
	aiBufferRatio      = 0.2
	aiMaxAnswerRounds  = 3
	aiMaxAgingItems    = 8
	aiDateLayout       = "2006-01-02"

	aiRoleUser      = "user"
	aiRoleAssistant = "assistant"
)

// aiMandatoryTopics must be asked and answered for every item before planning.
var aiMandatoryTopics = []models.QuestionTopic{models.TopicBlockers, models.TopicEstimate, models.TopicBuffer}

type AIService struct {
	client       gemini.Client
	planningRepo repository.PlanningRepository
	summaryRepo  repository.SummaryRepository
	sprintRepo   repository.SprintRepository
	analytics    Analytics
	tasks        Task
	logger       *zap.Logger
}

func NewAIService(
	client gemini.Client,
	planningRepo repository.PlanningRepository,
	summaryRepo repository.SummaryRepository,
	sprintRepo repository.SprintRepository,
	analytics Analytics,
	tasks Task,
	logger *zap.Logger,
) *AIService {
	return &AIService{
		client:       client,
		planningRepo: planningRepo,
		summaryRepo:  summaryRepo,
		sprintRepo:   sprintRepo,
		analytics:    analytics,
		tasks:        tasks,
		logger:       logger,
	}
}

func (a *AIService) Enabled() bool { return a.client != nil }

func (a *AIService) Model() string {
	if !a.Enabled() {
		return ""
	}
	return a.client.Model()
}

func (a *AIService) Summary(ctx context.Context, user *models.User, q models.AnalyticsQuery, lang string, refresh bool) (*models.AISummary, error) {
	if !a.Enabled() {
		return nil, apperrors.ErrAIDisabled
	}
	if user == nil {
		return nil, apperrors.ErrUnauthorized
	}
	lang = aiLang(lang)

	dashboard, err := a.analytics.Dashboard(ctx, user.ID, q)
	if err != nil {
		return nil, err
	}

	fingerprint := aiSummaryFingerprint(user.ID, q, lang, dashboard)
	if !refresh {
		cached, err := a.summaryRepo.GetByFingerprint(ctx, user.ID, fingerprint)
		if err != nil {
			a.logger.Error("failed to read cached ai summary", zap.Error(err), zap.String("user_id", user.ID.String()))
			return nil, fmt.Errorf("get cached ai summary: %w", err)
		}
		if cached != nil {
			cached.Cached = true
			return cached, nil
		}
	}

	view, err := json.Marshal(aiDashboardView(dashboard))
	if err != nil {
		a.logger.Error("failed to encode dashboard for ai summary", zap.Error(err))
		return nil, fmt.Errorf("encode dashboard: %w", err)
	}

	var content models.AISummaryContent
	if err := a.generate(ctx, "summary",
		fmt.Sprintf(aiSummarySystem, aiLangLine(lang)),
		fmt.Sprintf(aiSummaryPrompt, view),
		aiSummarySchema(), &content); err != nil {
		return nil, err
	}
	aiNormalizeSummary(&content)

	now := time.Now()
	summary := &models.AISummary{
		ID:          uuid.New(),
		UserID:      user.ID,
		Period:      string(q.Period),
		RangeStart:  q.From,
		RangeEnd:    q.To,
		Fingerprint: fingerprint,
		AIModel:     aiPtr(a.Model()),
		Content:     content,
		CreatedAt:   now,
	}

	if err := a.summaryRepo.SaveSummary(ctx, summary); err != nil {
		a.logger.Error("failed to save ai summary", zap.Error(err), zap.String("user_id", user.ID.String()))
		return nil, fmt.Errorf("save ai summary: %w", err)
	}

	summary.Cached = false
	return summary, nil
}

func (a *AIService) ListSummaries(ctx context.Context, userID uuid.UUID, limit int) ([]models.AISummary, error) {
	if !a.Enabled() {
		return nil, apperrors.ErrAIDisabled
	}

	summaries, err := a.summaryRepo.ListSummaries(ctx, userID, limit)
	if err != nil {
		a.logger.Error("failed to list ai summaries", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("list ai summaries: %w", err)
	}
	return summaries, nil
}

func (a *AIService) StartPlanning(ctx context.Context, userID uuid.UUID, in PlanningStart) (*models.PlanningSession, error) {
	if !a.Enabled() {
		return nil, apperrors.ErrAIDisabled
	}
	lang := aiLang(in.Lang)

	items := make([]models.PlanningItem, 0, aiMaxPlanningItems)
	for _, raw := range in.RawTasks {
		title := strings.TrimSpace(raw)
		if title == "" {
			continue
		}
		if len(items) >= aiMaxPlanningItems {
			break
		}
		items = append(items, models.PlanningItem{Title: title})
	}
	if len(items) == 0 {
		return nil, apperrors.ErrNoPlanningItems
	}

	if in.IncludeBacklog && len(items) < aiMaxPlanningItems {
		backlog, err := a.tasks.GetTasks(ctx, userID, models.TaskFilter{Statuses: []models.TaskStatus{models.StatusTodo}})
		if err != nil {
			return nil, err
		}
		for i := range backlog {
			if len(items) >= aiMaxPlanningItems {
				break
			}
			task := backlog[i]
			item := models.PlanningItem{
				Title:          task.Title,
				Priority:       task.Priority,
				ExistingTaskID: &task.ID,
			}
			if task.Description != nil {
				item.Notes = strings.TrimSpace(*task.Description)
			}
			items = append(items, item)
		}
	}

	for i := range items {
		items[i].Ref = "t" + strconv.Itoa(i+1)
	}

	horizon := aiClampInt(in.HorizonWeeks, 1, aiMaxHorizonWeeks)
	capacity := in.CapacityHoursPerWeek
	if capacity <= 0 {
		capacity = aiDefaultCapacity
	}
	capacity = aiRound2(aiClampFloat(capacity, 1, aiMaxCapacityHours))
	startsOn := aiStartDate(in.StartsOn)
	notes := strings.TrimSpace(in.Notes)

	var parsed aiItemsQuestions
	if err := a.generate(ctx, "planning_questions",
		fmt.Sprintf(aiPlanningStartSystem, aiLangLine(lang)),
		fmt.Sprintf(aiPlanningStartPrompt, horizon, capacity, startsOn.Format(aiDateLayout), aiOrDash(notes), aiItemsJSON(items)),
		aiQuestionsSchema(), &parsed); err != nil {
		return nil, err
	}

	byRef := make(map[string]int, len(items))
	for i, item := range items {
		byRef[item.Ref] = i
	}
	for _, enriched := range parsed.Items {
		idx, ok := byRef[strings.TrimSpace(enriched.Ref)]
		if !ok {
			continue
		}
		if priority := models.TaskPriority(strings.ToUpper(strings.TrimSpace(enriched.Priority))); priority.IsValid() {
			items[idx].Priority = priority
		}
		if items[idx].Notes == "" {
			items[idx].Notes = strings.TrimSpace(enriched.Notes)
		}
	}

	questions := aiBuildQuestions(items, parsed.Questions, lang)
	now := time.Now()
	session := &models.PlanningSession{
		ID:                   uuid.New(),
		UserID:               userID,
		State:                models.PlanningCollecting,
		HorizonWeeks:         horizon,
		CapacityHoursPerWeek: capacity,
		StartsOn:             &startsOn,
		AIModel:              aiPtr(a.Model()),
		Payload: models.PlanningPayload{
			Items:     items,
			Questions: questions,
			Notes:     notes,
			Messages: []models.PlanningMessage{{
				Role:    aiRoleAssistant,
				Content: aiIntroMessage(lang, len(items), len(questions)),
				At:      now,
			}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := a.planningRepo.CreateSession(ctx, session); err != nil {
		a.logger.Error("failed to create planning session", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("create planning session: %w", err)
	}

	return session, nil
}

func (a *AIService) AnswerPlanning(ctx context.Context, userID, sessionID uuid.UUID, answers []PlanningAnswer) (*models.PlanningSession, error) {
	if !a.Enabled() {
		return nil, apperrors.ErrAIDisabled
	}

	session, err := a.loadSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.State != models.PlanningCollecting && session.State != models.PlanningReady {
		return nil, apperrors.ErrSessionState
	}

	payload := &session.Payload
	lang := aiSessionLang(payload)
	now := time.Now()

	byID := make(map[string]int, len(payload.Questions))
	for i, question := range payload.Questions {
		byID[question.ID] = i
	}

	recorded := make([]string, 0, len(answers))
	for _, answer := range answers {
		idx, ok := byID[strings.TrimSpace(answer.QuestionID)]
		text := strings.TrimSpace(answer.Answer)
		if !ok || text == "" {
			continue
		}
		payload.Questions[idx].Answer = text
		payload.Questions[idx].AnsweredAt = &now
		recorded = append(recorded, payload.Questions[idx].Question+" - "+text)
	}
	payload.Messages = append(payload.Messages, models.PlanningMessage{
		Role:    aiRoleUser,
		Content: strings.Join(recorded, "\n"),
		At:      now,
	})

	round := 0
	for _, message := range payload.Messages {
		if message.Role == aiRoleUser {
			round++
		}
	}
	lastRound := round > aiMaxAnswerRounds

	var parsed aiExtractResponse
	if err := a.generate(ctx, "planning_extract",
		fmt.Sprintf(aiPlanningExtractSystem, aiLangLine(lang)),
		fmt.Sprintf(aiPlanningExtractPrompt, aiItemsJSON(payload.Items), aiTranscriptJSON(payload.Questions)),
		aiExtractSchema(), &parsed); err != nil {
		return nil, err
	}

	extracted := make(map[string]aiExtractedItem, len(parsed.Items))
	for _, item := range parsed.Items {
		extracted[strings.TrimSpace(item.Ref)] = item
	}
	refs := make(map[string]bool, len(payload.Items))
	for _, item := range payload.Items {
		refs[item.Ref] = true
	}

	for i := range payload.Items {
		item := &payload.Items[i]
		aiMergeItem(item, extracted[item.Ref], refs)
		item.Resolved = item.EstimateHours != nil && *item.EstimateHours > 0 &&
			aiMandatoryAnswered(item.Ref, payload.Questions)

		if lastRound && !item.Resolved {
			if item.EstimateHours == nil || *item.EstimateHours <= 0 {
				item.EstimateHours = aiPtr(aiDefaultEstimate)
			}
			item.Resolved = true
		}
		if item.BufferHours == nil && item.EstimateHours != nil {
			item.BufferHours = aiPtr(aiRoundQuarter(*item.EstimateHours * aiBufferRatio))
		}
	}

	if !lastRound {
		payload.Questions = append(payload.Questions, aiFollowUps(payload.Items, payload.Questions, parsed.FollowUpQuestions)...)
	}

	session.State = models.PlanningCollecting
	if aiAllResolved(payload.Items) {
		session.State = models.PlanningReady
	}
	session.AIModel = aiPtr(a.Model())
	session.UpdatedAt = now

	if err := a.planningRepo.UpdateSession(ctx, session); err != nil {
		a.logger.Error("failed to update planning session", zap.Error(err), zap.String("session_id", sessionID.String()))
		return nil, fmt.Errorf("update planning session: %w", err)
	}

	return session, nil
}

func (a *AIService) GeneratePlan(ctx context.Context, userID, sessionID uuid.UUID) (*models.PlanningSession, error) {
	if !a.Enabled() {
		return nil, apperrors.ErrAIDisabled
	}

	session, err := a.loadSession(ctx, userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.State != models.PlanningReady && session.State != models.PlanningPlanned {
		return nil, apperrors.ErrSessionState
	}

	payload := &session.Payload
	if len(payload.Items) == 0 {
		return nil, apperrors.ErrNoPlanningItems
	}

	lang := aiSessionLang(payload)
	horizon := aiClampInt(session.HorizonWeeks, 1, aiMaxHorizonWeeks)
	capacity := session.CapacityHoursPerWeek
	if capacity <= 0 {
		capacity = aiDefaultCapacity
	}
	capacity = aiRound2(aiClampFloat(capacity, 1, aiMaxCapacityHours))
	start := aiStartDate(session.StartsOn)

	var parsed aiPlanResponse
	if err := a.generate(ctx, "planning_plan",
		fmt.Sprintf(aiPlanSystem, aiLangLine(lang)),
		fmt.Sprintf(aiPlanPrompt, horizon, capacity, aiWeekWindows(start, horizon), aiResolvedItemsJSON(payload.Items), aiOrDash(payload.Notes)),
		aiPlanSchema(), &parsed); err != nil {
		return nil, err
	}

	plan := aiBuildPlan(parsed, payload.Items, start, horizon, capacity)
	payload.Plan = plan
	session.State = models.PlanningPlanned
	session.AIModel = aiPtr(a.Model())
	session.UpdatedAt = time.Now()

	if err := a.planningRepo.UpdateSession(ctx, session); err != nil {
		a.logger.Error("failed to save generated plan", zap.Error(err), zap.String("session_id", sessionID.String()))
		return nil, fmt.Errorf("update planning session: %w", err)
	}

	return session, nil
}

func (a *AIService) CommitPlan(ctx context.Context, userID, sessionID uuid.UUID) (*models.Sprint, []models.Task, error) {
	if !a.Enabled() {
		return nil, nil, apperrors.ErrAIDisabled
	}

	session, err := a.loadSession(ctx, userID, sessionID)
	if err != nil {
		return nil, nil, err
	}
	if session.State != models.PlanningPlanned || session.Payload.Plan == nil {
		return nil, nil, apperrors.ErrSessionState
	}

	plan := session.Payload.Plan
	horizon := aiClampInt(session.HorizonWeeks, 1, aiMaxHorizonWeeks)
	fallbackStart := aiStartDate(session.StartsOn)

	startsOn, err := time.Parse(aiDateLayout, plan.StartsOn)
	if err != nil {
		startsOn = fallbackStart
	}
	endsOn, err := time.Parse(aiDateLayout, plan.EndsOn)
	if err != nil || endsOn.Before(startsOn) {
		endsOn = startsOn.AddDate(0, 0, 7*horizon-1)
	}

	now := time.Now()
	capacity := plan.CapacityHours
	sprint := &models.Sprint{
		ID:            uuid.New(),
		UserID:        userID,
		Name:          plan.Name,
		StartsOn:      startsOn,
		EndsOn:        endsOn,
		CapacityHours: &capacity,
		Status:        models.SprintPlanned,
		AIModel:       aiPtr(a.Model()),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if goal := strings.TrimSpace(plan.Goal); goal != "" {
		sprint.Goal = &goal
	}
	if rationale := strings.TrimSpace(plan.Rationale); rationale != "" {
		sprint.AIRationale = &rationale
	}

	if err := a.sprintRepo.CreateSprint(ctx, sprint); err != nil {
		a.logger.Error("failed to create sprint from plan", zap.Error(err), zap.String("session_id", sessionID.String()))
		return nil, nil, fmt.Errorf("create sprint: %w", err)
	}

	tasks, err := a.tasks.CreateTasksFromPlan(ctx, userID, &sprint.ID, plan)
	if err != nil {
		if delErr := a.sprintRepo.DeleteSprint(ctx, userID, sprint.ID); delErr != nil {
			a.logger.Error("failed to roll back sprint after task creation failed",
				zap.Error(delErr), zap.String("sprint_id", sprint.ID.String()))
		}
		return nil, nil, err
	}

	committed := make([]string, 0, len(tasks))
	for _, task := range tasks {
		committed = append(committed, task.ID.String())
	}
	session.SprintID = &sprint.ID
	session.State = models.PlanningCommitted
	session.Payload.CommittedIDs = committed
	session.UpdatedAt = time.Now()

	if err := a.planningRepo.UpdateSession(ctx, session); err != nil {
		a.logger.Error("failed to mark planning session committed", zap.Error(err), zap.String("session_id", sessionID.String()))
		return nil, nil, fmt.Errorf("update planning session: %w", err)
	}

	return sprint, tasks, nil
}

func (a *AIService) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.PlanningSession, error) {
	if !a.Enabled() {
		return nil, apperrors.ErrAIDisabled
	}
	return a.loadSession(ctx, userID, sessionID)
}

func (a *AIService) ListSessions(ctx context.Context, userID uuid.UUID, limit int) ([]models.PlanningSession, error) {
	if !a.Enabled() {
		return nil, apperrors.ErrAIDisabled
	}

	sessions, err := a.planningRepo.ListSessions(ctx, userID, limit)
	if err != nil {
		a.logger.Error("failed to list planning sessions", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("list planning sessions: %w", err)
	}
	return sessions, nil
}

func (a *AIService) DeleteSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	if !a.Enabled() {
		return apperrors.ErrAIDisabled
	}

	if err := a.planningRepo.DeleteSession(ctx, userID, sessionID); err != nil {
		if errors.Is(err, apperrors.ErrSessionNotFound) {
			return err
		}
		a.logger.Error("failed to delete planning session", zap.Error(err), zap.String("session_id", sessionID.String()))
		return fmt.Errorf("delete planning session: %w", err)
	}
	return nil
}

func (a *AIService) loadSession(ctx context.Context, userID, sessionID uuid.UUID) (*models.PlanningSession, error) {
	session, err := a.planningRepo.GetSession(ctx, userID, sessionID)
	if err != nil {
		if errors.Is(err, apperrors.ErrSessionNotFound) {
			return nil, err
		}
		a.logger.Error("failed to load planning session", zap.Error(err), zap.String("session_id", sessionID.String()))
		return nil, fmt.Errorf("get planning session: %w", err)
	}
	return session, nil
}

// generate runs one schema-constrained Gemini call and decodes the model output
// into out. Every provider failure is logged here and mapped to a sentinel.
func (a *AIService) generate(ctx context.Context, op, system, prompt string, schema map[string]any, out any) error {
	raw, err := a.client.GenerateJSON(ctx, gemini.Request{System: system, Prompt: prompt, Schema: schema})
	if err != nil {
		switch {
		case errors.Is(err, gemini.ErrTransient):
			a.logger.Error("gemini call failed", zap.String("op", op), zap.Error(err))
			return apperrors.ErrAIUnavailable
		case errors.Is(err, gemini.ErrBadResponse):
			a.logger.Error("gemini returned an unusable response", zap.String("op", op), zap.Error(err))
			return apperrors.ErrAIBadResponse
		default:
			a.logger.Error("gemini call failed", zap.String("op", op), zap.Error(err))
			return fmt.Errorf("gemini %s: %w", op, err)
		}
	}

	if err := json.Unmarshal(raw, out); err != nil {
		a.logger.Error("failed to decode gemini payload", zap.String("op", op), zap.Error(err))
		return apperrors.ErrAIBadResponse
	}
	return nil
}

type aiItemPayload struct {
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Priority string `json:"priority"`
	Notes    string `json:"notes"`
}

type aiQuestionPayload struct {
	ID          string   `json:"id"`
	ItemRef     string   `json:"item_ref"`
	Topic       string   `json:"topic"`
	Question    string   `json:"question"`
	Why         string   `json:"why"`
	Suggestions []string `json:"suggestions"`
}

type aiItemsQuestions struct {
	Items     []aiItemPayload     `json:"items"`
	Questions []aiQuestionPayload `json:"questions"`
}

type aiExtractedItem struct {
	Ref           string   `json:"ref"`
	Priority      string   `json:"priority"`
	HasBlockers   *bool    `json:"has_blockers"`
	Blockers      string   `json:"blockers"`
	EstimateHours *float64 `json:"estimate_hours"`
	BufferHours   *float64 `json:"buffer_hours"`
	NeedsReview   *bool    `json:"needs_review"`
	Reviewer      string   `json:"reviewer"`
	DependsOn     []string `json:"depends_on"`
	Resolved      bool     `json:"resolved"`
}

type aiExtractResponse struct {
	Items             []aiExtractedItem   `json:"items"`
	FollowUpQuestions []aiQuestionPayload `json:"follow_up_questions"`
	AllResolved       bool                `json:"all_resolved"`
}

type aiPlanResponse struct {
	Name  string `json:"name"`
	Goal  string `json:"goal"`
	Weeks []struct {
		Index int    `json:"index"`
		Focus string `json:"focus"`
		Tasks []struct {
			Ref           string   `json:"ref"`
			Title         string   `json:"title"`
			Description   string   `json:"description"`
			Priority      string   `json:"priority"`
			EstimateHours float64  `json:"estimate_hours"`
			BufferHours   float64  `json:"buffer_hours"`
			Blockers      string   `json:"blockers"`
			NeedsReview   bool     `json:"needs_review"`
			Reviewer      string   `json:"reviewer"`
			DependsOn     []string `json:"depends_on"`
			Subtasks      []struct {
				Title         string  `json:"title"`
				EstimateHours float64 `json:"estimate_hours"`
			} `json:"subtasks"`
		} `json:"tasks"`
	} `json:"weeks"`
	Deferred []struct {
		Ref    string `json:"ref"`
		Title  string `json:"title"`
		Reason string `json:"reason"`
	} `json:"deferred"`
	Risks           []string `json:"risks"`
	Rationale       string   `json:"rationale"`
	Recommendations []string `json:"recommendations"`
}

// aiDashboardView is the trimmed projection of the dashboard sent to the model:
// the dense series, heatmap and weekday breakdowns would blow up the prompt.
func aiDashboardView(d *models.Dashboard) any {
	type seriesPoint struct {
		Label     string `json:"label"`
		Created   int    `json:"created"`
		Completed int    `json:"completed"`
	}
	type agingRow struct {
		Title     string              `json:"title"`
		Status    models.TaskStatus   `json:"status"`
		Priority  models.TaskPriority `json:"priority"`
		AgeHours  float64             `json:"age_hours"`
		IsOverdue bool                `json:"is_overdue"`
	}

	series := make([]seriesPoint, 0, len(d.Series))
	for _, bucket := range d.Series {
		series = append(series, seriesPoint{Label: bucket.Label, Created: bucket.Created, Completed: bucket.Completed})
	}

	aging := make([]agingRow, 0, aiMaxAgingItems)
	for _, item := range d.AgingWIP {
		if len(aging) >= aiMaxAgingItems {
			break
		}
		aging = append(aging, agingRow{
			Title:     item.Title,
			Status:    item.Status,
			Priority:  item.Priority,
			AgeHours:  aiRound2(item.AgeHours),
			IsOverdue: item.IsOverdue,
		})
	}

	return struct {
		Range      models.RangeInfo        `json:"range"`
		Totals     models.Totals           `json:"totals"`
		Comparison models.Comparison       `json:"comparison"`
		Series     []seriesPoint           `json:"series"`
		Priority   []models.PriorityBucket `json:"priority_breakdown"`
		Status     []models.StatusBucket   `json:"status_breakdown"`
		Aging      []agingRow              `json:"aging_wip"`
		Reviewers  []models.ReviewerBucket `json:"reviewers"`
	}{
		Range:      d.Range,
		Totals:     d.Totals,
		Comparison: d.Comparison,
		Series:     series,
		Priority:   d.PriorityBreakdown,
		Status:     d.StatusBreakdown,
		Aging:      aging,
		Reviewers:  d.Reviewers,
	}
}

// aiSummaryFingerprint is stable for identical inputs, which is what makes the
// summary cache hit instead of paying for another generation.
func aiSummaryFingerprint(userID uuid.UUID, q models.AnalyticsQuery, lang string, d *models.Dashboard) string {
	t := d.Totals
	canonical := strings.Join([]string{
		userID.String(),
		string(q.Period),
		q.From.UTC().Format(time.RFC3339),
		q.To.UTC().Format(time.RFC3339),
		string(q.Granularity),
		lang,
		aiFormat2(float64(t.CreatedInRange)),
		aiFormat2(float64(t.CompletedInRange)),
		aiFormat2(float64(t.OpenNow)),
		aiFormat2(t.CompletionRate),
		aiFormat2(t.AvgCycleTimeHours),
		aiFormat2(float64(t.Overdue)),
		aiFormat2(float64(t.TotalTasks)),
	}, "|")

	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:])
}

func aiNormalizeSummary(c *models.AISummaryContent) {
	c.Headline = strings.TrimSpace(c.Headline)
	c.Summary = strings.TrimSpace(c.Summary)
	c.Highlights = aiCleanStrings(c.Highlights, 5)
	c.Risks = aiCleanStrings(c.Risks, 4)
	c.Recommendations = aiCleanStrings(c.Recommendations, 4)
	c.FocusNext = aiCleanStrings(c.FocusNext, 4)
	c.Score = aiClampInt(c.Score, 0, 100)

	switch strings.ToUpper(strings.TrimSpace(c.Trend)) {
	case "IMPROVING":
		c.Trend = "IMPROVING"
	case "DECLINING":
		c.Trend = "DECLINING"
	case "AT_RISK":
		c.Trend = "AT_RISK"
	default:
		c.Trend = "STEADY"
	}

	metrics := make([]models.AIMetricNote, 0, len(c.Metrics))
	for _, metric := range c.Metrics {
		label := strings.TrimSpace(metric.Label)
		value := strings.TrimSpace(metric.Value)
		if label == "" || value == "" || len(metrics) >= 6 {
			continue
		}
		metrics = append(metrics, models.AIMetricNote{Label: label, Value: value, Comment: strings.TrimSpace(metric.Comment)})
	}
	c.Metrics = metrics
}

// aiBuildQuestions guarantees the three mandatory topics per item, unique ids
// and a deterministic order, whatever the model actually returned.
func aiBuildQuestions(items []models.PlanningItem, raw []aiQuestionPayload, lang string) []models.PlanningQuestion {
	known := make(map[string]bool, len(items))
	for _, item := range items {
		known[item.Ref] = true
	}

	taken := make(map[string]bool, len(raw))
	counter := 0
	byRef := make(map[string][]models.PlanningQuestion, len(items))
	for _, rq := range raw {
		ref := strings.TrimSpace(rq.ItemRef)
		text := strings.TrimSpace(rq.Question)
		if !known[ref] || text == "" {
			continue
		}
		byRef[ref] = append(byRef[ref], models.PlanningQuestion{
			ID:          aiUniqueID(rq.ID, taken, &counter),
			ItemRef:     ref,
			Topic:       aiTopic(rq.Topic),
			Question:    text,
			Why:         strings.TrimSpace(rq.Why),
			Suggestions: aiCleanStrings(rq.Suggestions, 4),
		})
	}

	questions := make([]models.PlanningQuestion, 0, len(items)*len(aiMandatoryTopics))
	for _, item := range items {
		bucket := byRef[item.Ref]
		used := make([]bool, len(bucket))
		for _, topic := range aiMandatoryTopics {
			matched := false
			for i := range bucket {
				if !used[i] && bucket[i].Topic == topic {
					used[i] = true
					questions = append(questions, bucket[i])
					matched = true
					break
				}
			}
			if !matched {
				questions = append(questions, aiFallbackQuestion(item, topic, lang, taken, &counter))
			}
		}
		for i := range bucket {
			if !used[i] {
				questions = append(questions, bucket[i])
			}
		}
	}
	return questions
}

func aiFollowUps(items []models.PlanningItem, existing []models.PlanningQuestion, raw []aiQuestionPayload) []models.PlanningQuestion {
	resolved := make(map[string]bool, len(items))
	for _, item := range items {
		resolved[item.Ref] = item.Resolved
	}
	taken := make(map[string]bool, len(existing))
	for _, question := range existing {
		taken[question.ID] = true
	}

	counter := 0
	asked := make(map[string]bool, len(items))
	followUps := make([]models.PlanningQuestion, 0, len(raw))
	for _, rq := range raw {
		ref := strings.TrimSpace(rq.ItemRef)
		text := strings.TrimSpace(rq.Question)
		isResolved, known := resolved[ref]
		if !known || isResolved || text == "" || asked[ref] {
			continue
		}
		asked[ref] = true
		followUps = append(followUps, models.PlanningQuestion{
			ID:          aiUniqueID(rq.ID, taken, &counter),
			ItemRef:     ref,
			Topic:       aiTopic(rq.Topic),
			Question:    text,
			Why:         strings.TrimSpace(rq.Why),
			Suggestions: aiCleanStrings(rq.Suggestions, 4),
		})
	}
	return followUps
}

// aiMergeItem folds the extracted values onto the item without ever replacing
// something the user already supplied with an empty model value.
func aiMergeItem(item *models.PlanningItem, extracted aiExtractedItem, refs map[string]bool) {
	if priority := models.TaskPriority(strings.ToUpper(strings.TrimSpace(extracted.Priority))); priority.IsValid() {
		item.Priority = priority
	}
	if extracted.HasBlockers != nil {
		item.HasBlockers = extracted.HasBlockers
	}
	if blockers := strings.TrimSpace(extracted.Blockers); blockers != "" {
		item.Blockers = blockers
	}
	if extracted.EstimateHours != nil && *extracted.EstimateHours > 0 {
		item.EstimateHours = aiPtr(aiRound2(aiClampFloat(*extracted.EstimateHours, 0, aiMaxTaskHours)))
	}
	if extracted.BufferHours != nil && *extracted.BufferHours >= 0 {
		item.BufferHours = aiPtr(aiRound2(aiClampFloat(*extracted.BufferHours, 0, aiMaxTaskHours)))
	}
	if extracted.NeedsReview != nil {
		item.NeedsReview = extracted.NeedsReview
	}
	if reviewer := strings.TrimSpace(extracted.Reviewer); reviewer != "" {
		item.Reviewer = reviewer
	}

	depends := make([]string, 0, len(extracted.DependsOn))
	for _, ref := range extracted.DependsOn {
		ref = strings.TrimSpace(ref)
		if ref != "" && ref != item.Ref && refs[ref] {
			depends = append(depends, ref)
		}
	}
	if len(depends) > 0 {
		item.DependsOn = depends
	}
}

func aiMandatoryAnswered(ref string, questions []models.PlanningQuestion) bool {
	for _, topic := range aiMandatoryTopics {
		answered := false
		for _, question := range questions {
			if question.ItemRef == ref && question.Topic == topic && strings.TrimSpace(question.Answer) != "" {
				answered = true
				break
			}
		}
		if !answered {
			return false
		}
	}
	return true
}

func aiAllResolved(items []models.PlanningItem) bool {
	for _, item := range items {
		if !item.Resolved {
			return false
		}
	}
	return len(items) > 0
}

// aiBuildPlan rebuilds the plan from the model proposal: the week windows, the
// ordering and every total are computed here, never taken from the model.
func aiBuildPlan(parsed aiPlanResponse, items []models.PlanningItem, start time.Time, horizon int, capacity float64) *models.SprintPlan {
	byRef := make(map[string]models.PlanningItem, len(items))
	for _, item := range items {
		byRef[item.Ref] = item
	}

	plan := &models.SprintPlan{
		Name:            strings.TrimSpace(parsed.Name),
		Goal:            strings.TrimSpace(parsed.Goal),
		StartsOn:        start.Format(aiDateLayout),
		EndsOn:          start.AddDate(0, 0, 7*horizon-1).Format(aiDateLayout),
		CapacityHours:   aiRound2(capacity * float64(horizon)),
		Weeks:           make([]models.PlannedWeek, 0, horizon),
		Risks:           aiCleanStrings(parsed.Risks, 6),
		Rationale:       strings.TrimSpace(parsed.Rationale),
		Recommendations: aiCleanStrings(parsed.Recommendations, 6),
	}
	if plan.Name == "" {
		plan.Name = "Sprint " + plan.StartsOn
	}

	scheduled := make(map[string]bool, len(items))
	for index := 0; index < horizon; index++ {
		weekStart := start.AddDate(0, 0, 7*index)
		week := models.PlannedWeek{
			Index:         index + 1,
			StartsOn:      weekStart.Format(aiDateLayout),
			EndsOn:        weekStart.AddDate(0, 0, 6).Format(aiDateLayout),
			CapacityHours: capacity,
			Tasks:         []models.PlannedTask{},
		}

		if index < len(parsed.Weeks) {
			source := parsed.Weeks[index]
			week.Focus = strings.TrimSpace(source.Focus)
			for _, raw := range source.Tasks {
				ref := strings.TrimSpace(raw.Ref)
				item, ok := byRef[ref]
				if !ok || scheduled[ref] {
					continue
				}
				scheduled[ref] = true

				task := models.PlannedTask{
					Ref:           ref,
					Title:         aiFirstNonEmpty(raw.Title, item.Title),
					Description:   aiFirstNonEmpty(raw.Description, item.Notes),
					Priority:      models.TaskPriority(strings.ToUpper(strings.TrimSpace(raw.Priority))),
					EstimateHours: aiRound2(aiClampFloat(raw.EstimateHours, 0, aiMaxTaskHours)),
					BufferHours:   aiRound2(aiClampFloat(raw.BufferHours, 0, aiMaxTaskHours)),
					Blockers:      aiFirstNonEmpty(raw.Blockers, item.Blockers),
					NeedsReview:   raw.NeedsReview || (item.NeedsReview != nil && *item.NeedsReview),
					Reviewer:      aiFirstNonEmpty(raw.Reviewer, item.Reviewer),
					Order:         len(week.Tasks) + 1,
				}
				if !task.Priority.IsValid() {
					task.Priority = item.Priority
					if !task.Priority.IsValid() {
						task.Priority = models.PriorityMedium
					}
				}
				if task.EstimateHours <= 0 && item.EstimateHours != nil {
					task.EstimateHours = aiRound2(aiClampFloat(*item.EstimateHours, 0, aiMaxTaskHours))
				}
				if task.BufferHours <= 0 && item.BufferHours != nil {
					task.BufferHours = aiRound2(aiClampFloat(*item.BufferHours, 0, aiMaxTaskHours))
				}

				for _, dep := range aiCleanStrings(raw.DependsOn, len(items)) {
					if _, known := byRef[dep]; known && dep != ref {
						task.DependsOn = append(task.DependsOn, dep)
					}
				}
				for _, sub := range raw.Subtasks {
					title := strings.TrimSpace(sub.Title)
					if title == "" || len(task.Subtasks) >= 5 {
						continue
					}
					task.Subtasks = append(task.Subtasks, models.PlannedSubtask{
						Title:         title,
						EstimateHours: aiRound2(aiClampFloat(sub.EstimateHours, 0, aiMaxTaskHours)),
					})
				}

				week.LoadHours = aiRound2(week.LoadHours + task.EstimateHours + task.BufferHours)
				plan.TotalEstimateHours = aiRound2(plan.TotalEstimateHours + task.EstimateHours)
				plan.TotalBufferHours = aiRound2(plan.TotalBufferHours + task.BufferHours)
				week.Tasks = append(week.Tasks, task)
			}
		}
		plan.Weeks = append(plan.Weeks, week)
	}

	for _, raw := range parsed.Deferred {
		ref := strings.TrimSpace(raw.Ref)
		item, ok := byRef[ref]
		if !ok || scheduled[ref] {
			continue
		}
		scheduled[ref] = true
		plan.Deferred = append(plan.Deferred, models.DeferredItem{
			Ref:    ref,
			Title:  aiFirstNonEmpty(raw.Title, item.Title),
			Reason: strings.TrimSpace(raw.Reason),
		})
	}

	plan.CommittedHours = aiRound2(plan.TotalEstimateHours + plan.TotalBufferHours)
	if plan.CapacityHours > 0 {
		plan.LoadPercent = aiRound1(plan.CommittedHours / plan.CapacityHours * 100)
	}
	return plan
}

func aiItemsJSON(items []models.PlanningItem) string {
	type view struct {
		Ref      string `json:"ref"`
		Title    string `json:"title"`
		Notes    string `json:"notes,omitempty"`
		Priority string `json:"priority,omitempty"`
	}

	rows := make([]view, 0, len(items))
	for _, item := range items {
		rows = append(rows, view{Ref: item.Ref, Title: item.Title, Notes: item.Notes, Priority: string(item.Priority)})
	}
	return aiJSON(rows)
}

func aiResolvedItemsJSON(items []models.PlanningItem) string {
	type view struct {
		Ref           string   `json:"ref"`
		Title         string   `json:"title"`
		Notes         string   `json:"notes,omitempty"`
		Priority      string   `json:"priority"`
		EstimateHours float64  `json:"estimate_hours"`
		BufferHours   float64  `json:"buffer_hours"`
		Blockers      string   `json:"blockers,omitempty"`
		NeedsReview   bool     `json:"needs_review"`
		Reviewer      string   `json:"reviewer,omitempty"`
		DependsOn     []string `json:"depends_on,omitempty"`
	}

	rows := make([]view, 0, len(items))
	for _, item := range items {
		row := view{
			Ref:           item.Ref,
			Title:         item.Title,
			Notes:         item.Notes,
			Priority:      string(models.PriorityMedium),
			EstimateHours: aiDefaultEstimate,
			Blockers:      item.Blockers,
			NeedsReview:   item.NeedsReview != nil && *item.NeedsReview,
			Reviewer:      item.Reviewer,
			DependsOn:     item.DependsOn,
		}
		if item.Priority.IsValid() {
			row.Priority = string(item.Priority)
		}
		if item.EstimateHours != nil && *item.EstimateHours > 0 {
			row.EstimateHours = *item.EstimateHours
		}
		if item.BufferHours != nil {
			row.BufferHours = *item.BufferHours
		} else {
			row.BufferHours = aiRoundQuarter(row.EstimateHours * aiBufferRatio)
		}
		rows = append(rows, row)
	}
	return aiJSON(rows)
}

func aiTranscriptJSON(questions []models.PlanningQuestion) string {
	type view struct {
		QuestionID string `json:"question_id"`
		ItemRef    string `json:"item_ref"`
		Topic      string `json:"topic"`
		Question   string `json:"question"`
		Answer     string `json:"answer"`
	}

	rows := make([]view, 0, len(questions))
	for _, question := range questions {
		rows = append(rows, view{
			QuestionID: question.ID,
			ItemRef:    question.ItemRef,
			Topic:      string(question.Topic),
			Question:   question.Question,
			Answer:     strings.TrimSpace(question.Answer),
		})
	}
	return aiJSON(rows)
}

func aiWeekWindows(start time.Time, horizon int) string {
	windows := make([]string, 0, horizon)
	for i := 0; i < horizon; i++ {
		weekStart := start.AddDate(0, 0, 7*i)
		windows = append(windows, fmt.Sprintf("week %d: %s..%s",
			i+1, weekStart.Format(aiDateLayout), weekStart.AddDate(0, 0, 6).Format(aiDateLayout)))
	}
	return strings.Join(windows, "\n")
}

func aiJSON(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// aiSessionLang recovers the answer language from what the assistant has
// already written, since the session itself carries no language field.
func aiSessionLang(payload *models.PlanningPayload) string {
	var sb strings.Builder
	for _, question := range payload.Questions {
		sb.WriteString(question.Question)
	}
	for _, message := range payload.Messages {
		sb.WriteString(message.Content)
	}
	for _, r := range sb.String() {
		if unicode.Is(unicode.Cyrillic, r) {
			return "uk"
		}
	}
	return "en"
}

func aiLang(lang string) string {
	if strings.EqualFold(strings.TrimSpace(lang), "en") {
		return "en"
	}
	return "uk"
}

func aiLangLine(lang string) string {
	if lang == "en" {
		return fmt.Sprintf(aiLangDirective, "English")
	}
	return fmt.Sprintf(aiLangDirective, "Ukrainian")
}

func aiStartDate(at *time.Time) time.Time {
	if at != nil {
		utc := at.UTC()
		return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	shift := (8 - int(today.Weekday())) % 7
	if shift == 0 {
		shift = 7
	}
	return today.AddDate(0, 0, shift)
}

func aiTopic(raw string) models.QuestionTopic {
	topic := models.QuestionTopic(strings.ToUpper(strings.TrimSpace(raw)))
	switch topic {
	case models.TopicBlockers, models.TopicEstimate, models.TopicBuffer,
		models.TopicScope, models.TopicPriority, models.TopicDependency, models.TopicReview:
		return topic
	}
	return models.TopicScope
}

func aiUniqueID(raw string, taken map[string]bool, counter *int) string {
	id := strings.TrimSpace(raw)
	if id != "" && !taken[id] {
		taken[id] = true
		return id
	}
	for {
		*counter++
		candidate := "q" + strconv.Itoa(*counter)
		if !taken[candidate] {
			taken[candidate] = true
			return candidate
		}
	}
}

func aiCleanStrings(values []string, max int) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(cleaned) >= max {
			continue
		}
		cleaned = append(cleaned, value)
	}
	if len(cleaned) == 0 {
		return nil
	}
	return cleaned
}

func aiFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func aiOrDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func aiClampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func aiClampFloat(value, min, max float64) float64 {
	if math.IsNaN(value) || value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func aiRound1(value float64) float64 { return math.Round(value*10) / 10 }

func aiRound2(value float64) float64 { return math.Round(value*100) / 100 }

func aiRoundQuarter(value float64) float64 { return math.Round(value*4) / 4 }

func aiFormat2(value float64) string { return strconv.FormatFloat(aiRound2(value), 'f', 2, 64) }

func aiPtr[T any](value T) *T { return &value }

func aiIntroMessage(lang string, items, questions int) string {
	if lang == "en" {
		return fmt.Sprintf("I took %d task(s) and prepared %d clarifying questions - blockers, effort estimate and safety buffer for each of them. Answer them and I will build the sprint plan.", items, questions)
	}
	return fmt.Sprintf("Я взяв %d задач(і) і підготував %d уточнювальних питань - блокери, оцінка часу та буфер для кожної. Дай відповіді, і я складу план спринту.", items, questions)
}

func aiFallbackQuestion(item models.PlanningItem, topic models.QuestionTopic, lang string, taken map[string]bool, counter *int) models.PlanningQuestion {
	question := models.PlanningQuestion{
		ID:      aiUniqueID(item.Ref+"-"+strings.ToLower(string(topic)), taken, counter),
		ItemRef: item.Ref,
		Topic:   topic,
	}

	english := lang == "en"
	switch topic {
	case models.TopicBlockers:
		if english {
			question.Question = fmt.Sprintf("Is anything blocking %q right now?", item.Title)
			question.Why = "Blocked work has to be scheduled late in the sprint or deferred."
			question.Suggestions = []string{"Nothing blocks it", "Waiting for a review", "Waiting for another team", "Blocked by another task on this list"}
		} else {
			question.Question = fmt.Sprintf("Чи щось блокує задачу %q зараз?", item.Title)
			question.Why = "Заблоковану роботу треба ставити в кінець спринту або відкладати."
			question.Suggestions = []string{"Нічого не блокує", "Чекаю на рев'ю", "Чекаю на іншу команду", "Залежить від іншої задачі зі списку"}
		}
	case models.TopicEstimate:
		if english {
			question.Question = fmt.Sprintf("How many hours of focused work will %q take?", item.Title)
			question.Why = "The estimate decides what fits into the weekly capacity."
			question.Suggestions = []string{"1-2 hours", "Half a day (4 hours)", "A full day (8 hours)", "Two to three days (16-24 hours)"}
		} else {
			question.Question = fmt.Sprintf("Скільки годин зосередженої роботи займе %q?", item.Title)
			question.Why = "Від оцінки залежить, що вміститься в тижневу місткість."
			question.Suggestions = []string{"1-2 години", "Пів дня (4 години)", "Повний день (8 годин)", "Два-три дні (16-24 години)"}
		}
	case models.TopicBuffer:
		if english {
			question.Question = fmt.Sprintf("How much safety buffer do you want on top of the estimate for %q?", item.Title)
			question.Why = "The buffer absorbs surprises without breaking the sprint."
			question.Suggestions = []string{"No buffer", "About 20% of the estimate", "Half a day extra", "A full day extra"}
		} else {
			question.Question = fmt.Sprintf("Який запас часу додати понад оцінку для %q?", item.Title)
			question.Why = "Буфер поглинає несподіванки й не ламає спринт."
			question.Suggestions = []string{"Без буфера", "Близько 20% від оцінки", "Пів дня зверху", "Повний день зверху"}
		}
	}
	return question
}

func aiSummarySchema() map[string]any {
	return map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"headline":        aiStringField("one sentence, the single most important fact about this window"),
			"summary":         aiStringField("2 to 4 sentences"),
			"highlights":      aiStringArray("2 to 5 concrete wins backed by the numbers"),
			"risks":           aiStringArray("0 to 4 risks visible in the numbers"),
			"recommendations": aiStringArray("2 to 4 actionable recommendations"),
			"focus_next":      aiStringArray("2 to 4 things to focus on next"),
			"metrics": map[string]any{
				"type":        "ARRAY",
				"description": "2 to 6 metrics worth calling out",
				"items": map[string]any{
					"type": "OBJECT",
					"properties": map[string]any{
						"label":   aiStringField(""),
						"value":   aiStringField("the value as it should be shown, e.g. 12 or 43.5%"),
						"comment": aiStringField("short reading of the value"),
					},
					"required": []string{"label", "value"},
				},
			},
			"trend": aiEnumField([]string{"IMPROVING", "STEADY", "DECLINING", "AT_RISK"}),
			"score": map[string]any{"type": "INTEGER", "description": "productivity score from 0 to 100"},
		},
		"required": []string{"headline", "summary", "highlights", "recommendations", "focus_next", "metrics", "trend", "score"},
	}
}

func aiQuestionsSchema() map[string]any {
	return map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "ARRAY",
				"items": map[string]any{
					"type": "OBJECT",
					"properties": map[string]any{
						"ref":      aiStringField("the ref exactly as given in the input"),
						"title":    aiStringField(""),
						"priority": aiEnumField([]string{"LOW", "MEDIUM", "HIGH", "URGENT"}),
						"notes":    aiStringField(""),
					},
					"required": []string{"ref", "title", "priority"},
				},
			},
			"questions": map[string]any{
				"type":        "ARRAY",
				"description": "at least one BLOCKERS, one ESTIMATE and one BUFFER question per item",
				"items":       aiQuestionItemSchema(),
			},
		},
		"required": []string{"items", "questions"},
	}
}

func aiExtractSchema() map[string]any {
	return map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "ARRAY",
				"items": map[string]any{
					"type": "OBJECT",
					"properties": map[string]any{
						"ref":            aiStringField("the ref exactly as given in the input"),
						"priority":       aiEnumField([]string{"LOW", "MEDIUM", "HIGH", "URGENT"}),
						"has_blockers":   map[string]any{"type": "BOOLEAN"},
						"blockers":       aiStringField("what blocks the task, empty when nothing does"),
						"estimate_hours": map[string]any{"type": "NUMBER", "description": "hours of focused work the user stated"},
						"buffer_hours":   map[string]any{"type": "NUMBER", "description": "safety buffer in hours the user asked for"},
						"needs_review":   map[string]any{"type": "BOOLEAN"},
						"reviewer":       aiStringField(""),
						"depends_on":     aiStringArray("refs this task depends on"),
						"resolved":       map[string]any{"type": "BOOLEAN", "description": "true when blockers, estimate and buffer are all known"},
					},
					"required": []string{"ref", "resolved"},
				},
			},
			"follow_up_questions": map[string]any{
				"type":        "ARRAY",
				"description": "at most one question per unresolved item",
				"items":       aiQuestionItemSchema(),
			},
			"all_resolved": map[string]any{"type": "BOOLEAN"},
		},
		"required": []string{"items", "all_resolved"},
	}
}

func aiPlanSchema() map[string]any {
	return map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"name": aiStringField("short sprint name"),
			"goal": aiStringField("one sentence sprint goal"),
			"weeks": map[string]any{
				"type": "ARRAY",
				"items": map[string]any{
					"type": "OBJECT",
					"properties": map[string]any{
						"index":     map[string]any{"type": "INTEGER", "description": "1-based week number"},
						"starts_on": aiStringField("YYYY-MM-DD"),
						"ends_on":   aiStringField("YYYY-MM-DD"),
						"focus":     aiStringField("what this week is about"),
						"tasks": map[string]any{
							"type": "ARRAY",
							"items": map[string]any{
								"type": "OBJECT",
								"properties": map[string]any{
									"ref":            aiStringField("the ref of the planned item"),
									"title":          aiStringField(""),
									"description":    aiStringField(""),
									"priority":       aiEnumField([]string{"LOW", "MEDIUM", "HIGH", "URGENT"}),
									"estimate_hours": map[string]any{"type": "NUMBER"},
									"buffer_hours":   map[string]any{"type": "NUMBER"},
									"blockers":       aiStringField(""),
									"needs_review":   map[string]any{"type": "BOOLEAN"},
									"reviewer":       aiStringField(""),
									"depends_on":     aiStringArray("refs that must be done first"),
									"order":          map[string]any{"type": "INTEGER", "description": "1-based order inside the week"},
									"subtasks": map[string]any{
										"type":        "ARRAY",
										"description": "2 to 5 subtasks, only for tasks longer than 8 hours",
										"items": map[string]any{
											"type": "OBJECT",
											"properties": map[string]any{
												"title":          aiStringField(""),
												"estimate_hours": map[string]any{"type": "NUMBER", "description": "at most 6"},
											},
											"required": []string{"title", "estimate_hours"},
										},
									},
								},
								"required": []string{"ref", "title", "priority", "estimate_hours", "buffer_hours", "order"},
							},
						},
					},
					"required": []string{"index", "starts_on", "ends_on", "tasks"},
				},
			},
			"deferred": map[string]any{
				"type": "ARRAY",
				"items": map[string]any{
					"type": "OBJECT",
					"properties": map[string]any{
						"ref":    aiStringField(""),
						"title":  aiStringField(""),
						"reason": aiStringField("why it does not fit into this sprint"),
					},
					"required": []string{"ref", "title", "reason"},
				},
			},
			"risks":           aiStringArray("risks of this plan"),
			"rationale":       aiStringField("why the work is ordered this way"),
			"recommendations": aiStringArray("what the user should do to make this plan hold"),
		},
		"required": []string{"name", "goal", "weeks", "rationale"},
	}
}

func aiQuestionItemSchema() map[string]any {
	return map[string]any{
		"type": "OBJECT",
		"properties": map[string]any{
			"id":          aiStringField("unique id of this question"),
			"item_ref":    aiStringField("the ref of the item this question is about"),
			"topic":       aiEnumField([]string{"BLOCKERS", "ESTIMATE", "BUFFER", "SCOPE", "PRIORITY", "DEPENDENCY", "REVIEW"}),
			"question":    aiStringField("one short question, never two questions in one sentence"),
			"why":         aiStringField("one short sentence on why this matters for the plan"),
			"suggestions": aiStringArray("2 to 4 concrete example answers"),
		},
		"required": []string{"id", "item_ref", "topic", "question", "suggestions"},
	}
}

func aiStringField(description string) map[string]any {
	field := map[string]any{"type": "STRING"}
	if description != "" {
		field["description"] = description
	}
	return field
}

func aiStringArray(description string) map[string]any {
	field := map[string]any{"type": "ARRAY", "items": map[string]any{"type": "STRING"}}
	if description != "" {
		field["description"] = description
	}
	return field
}

func aiEnumField(values []string) map[string]any {
	return map[string]any{"type": "STRING", "enum": values}
}

const aiLangDirective = "Write every word you output - headline, summary, questions, task titles, rationale, everything - in %s. Never mix languages."

const aiSummarySystem = `You are an engineering-productivity analyst writing for the single person whose task data this is.
Ground every claim in the numbers you are given and name the value you reason from.
Never invent a metric, a task or a date that is not in the input; if something cannot be read from the data, do not mention it.
Be concrete and short: no filler, no motivational padding, no generic advice.
Address the reader in the second person.
%s`

const aiSummaryPrompt = `Analytics window for this user as JSON:
%s

Review this window.`

const aiPlanningStartSystem = `You are an experienced tech lead running sprint planning with one engineer.
For EVERY task on the list you must find out three things: whether anything blocks it, how long it will take, and how much safety buffer to add on top of that estimate.
Ask one short question at a time. Never put two questions in one sentence and never ask about two tasks at once.
Ask extra questions beyond those three only when a task is genuinely ambiguous: unclear scope, an unnamed dependency, or a missing reviewer.
Give every question 2 to 4 concrete example answers as suggestions the engineer can pick from.
Keep every ref exactly as it appears in the input.
%s`

const aiPlanningStartPrompt = `Sprint horizon: %d week(s) at %.1f hours of capacity per week, starting %s.
User notes: %s

Tasks:
%s

Return the normalised items and the questions.`

const aiPlanningExtractSystem = `You are an experienced tech lead turning a planning conversation into structured data.
Extract per task only what the engineer actually said: priority, blockers, estimate in hours, buffer in hours, whether a review is needed and by whom, and dependencies on other refs.
Never invent numbers. Convert plain language into hours ("half a day" is 4, "a couple of days" is 16). If a range is given, take the upper bound.
An answer that denies something ("no blockers", "no buffer") is an explicit false or 0, not a missing value.
A task is resolved once its blockers, estimate and buffer are all known.
Ask a follow-up only when an answer is genuinely unusable: at most one per task, one short question, with 2 to 4 concrete suggestions.
%s`

const aiPlanningExtractPrompt = `Items:
%s

Full question and answer transcript:
%s

Extract the structured values and judge what is still unclear.`

const aiPlanSystem = `You are a pragmatic delivery lead building the sprint plan.
Follow these rules exactly:
- Never exceed the weekly capacity: the estimates plus buffers of the tasks placed in a week must fit inside that week's capacity.
- Blocked work goes late in the sprint, or into deferred with a concrete reason.
- The highest priority work goes first: URGENT before HIGH before MEDIUM before LOW.
- Break any task longer than 8 hours into 2 to 5 subtasks of at most 6 hours each, whose estimates sum to the task estimate.
- Every task keeps the engineer's own estimate and buffer unless it was split.
- Use only the refs you are given, each ref at most once, either inside a week or in deferred.
%s`

const aiPlanPrompt = `Sprint horizon: %d week(s) at %.1f hours of capacity per week.
Week windows:
%s

Resolved items:
%s

User notes: %s

Build the plan.`
