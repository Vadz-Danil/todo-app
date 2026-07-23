package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	maxTitleRunes    = 255
	maxReviewerRunes = 255
	maxTaskHours     = 1000
	planDateLayout   = "2006-01-02"
	planPositionStep = 1024
)


type TaskService struct {
	taskRepo   repository.TaskRepository
	sprintRepo repository.SprintRepository
	logger     *zap.Logger
}

func NewTaskService(taskRepo repository.TaskRepository, sprintRepo repository.SprintRepository, logger *zap.Logger) *TaskService {
	return &TaskService{
		taskRepo:   taskRepo,
		sprintRepo: sprintRepo,
		logger:     logger,
	}
}

// assertSprintOwned refuses a sprint_id that belongs to somebody else. The
// column is only guarded by a foreign key, so without this a caller could
// attach their own task to any sprint UUID they could guess.
func (s *TaskService) assertSprintOwned(ctx context.Context, userID uuid.UUID, sprintID *uuid.UUID) error {
	if sprintID == nil || s.sprintRepo == nil {
		return nil
	}

	if _, err := s.sprintRepo.GetSprintByID(ctx, userID, *sprintID); err != nil {
		if errors.Is(err, apperrors.ErrSprintNotFound) {
			return apperrors.ErrSprintNotOwned
		}
		s.logger.Error("failed to verify sprint ownership",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("sprint_id", sprintID.String()))
		return fmt.Errorf("verify sprint ownership: %w", err)
	}

	return nil
}

func (s *TaskService) CreateTask(ctx context.Context, userID uuid.UUID, in models.TaskCreate) (*models.Task, error) {
	if in.Status == "" {
		in.Status = models.StatusTodo
	}
	if in.Priority == "" {
		in.Priority = models.PriorityMedium
	}

	patch := models.TaskPatch{
		Title:         &in.Title,
		Description:   &in.Description,
		Status:        &in.Status,
		Priority:      &in.Priority,
		Reviewer:      &in.Reviewer,
		EstimateHours: &in.EstimateHours,
		BufferHours:   &in.BufferHours,
		Blockers:      &in.Blockers,
	}
	if err := normalizeTaskPatch(&patch, nil); err != nil {
		return nil, err
	}

	if err := s.assertSprintOwned(ctx, userID, in.SprintID); err != nil {
		return nil, err
	}

	position, err := s.taskRepo.AppendPosition(ctx, userID, in.Status)
	if err != nil {
		s.logger.Error("failed to compute append position", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("append position: %w", err)
	}

	now := time.Now().UTC()
	task := &models.Task{
		ID:            uuid.New(),
		UserID:        userID,
		Title:         *patch.Title,
		Description:   *patch.Description,
		Status:        *patch.Status,
		Priority:      *patch.Priority,
		Reviewer:      *patch.Reviewer,
		EstimateHours: *patch.EstimateHours,
		BufferHours:   *patch.BufferHours,
		Blockers:      *patch.Blockers,
		SprintID:      in.SprintID,
		DueDate:       in.DueDate,
		Position:      position,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if startsWork(task.Status) {
		task.StartedAt = &now
	}
	if task.Status == models.StatusDone {
		task.CompletedAt = &now
	}

	if err := s.taskRepo.CreateTask(ctx, task); err != nil {
		s.logger.Error("failed to create task", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("create task: %w", err)
	}

	s.recordStatusChange(ctx, task.ID, userID, nil, task.Status, now)

	return task, nil
}

func (s *TaskService) GetTasks(ctx context.Context, userID uuid.UUID, filter models.TaskFilter) ([]models.Task, error) {
	tasks, err := s.taskRepo.ListTasks(ctx, userID, filter)
	if err != nil {
		s.logger.Error("failed to list tasks", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	return tasks, nil
}

func (s *TaskService) GetTask(ctx context.Context, userID, taskID uuid.UUID) (*models.Task, error) {
	task, err := s.taskRepo.GetTaskByID(ctx, userID, taskID)
	if err != nil {
		if errors.Is(err, apperrors.ErrTaskNotFound) {
			return nil, err
		}
		s.logger.Error("failed to get task",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("task_id", taskID.String()))
		return nil, fmt.Errorf("get task: %w", err)
	}
	return task, nil
}

func (s *TaskService) UpdateTask(ctx context.Context, userID, taskID uuid.UUID, patch models.TaskPatch) (*models.Task, error) {
	current, err := s.GetTask(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}

	if err := normalizeTaskPatch(&patch, current); err != nil {
		return nil, err
	}

	if patch.SprintID != nil {
		if err := s.assertSprintOwned(ctx, userID, *patch.SprintID); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	statusChanged := applyTransitionTimestamps(&patch, current, now)

	updated, err := s.taskRepo.UpdateTask(ctx, userID, taskID, patch)
	if err != nil {
		if errors.Is(err, apperrors.ErrTaskNotFound) || errors.Is(err, apperrors.ErrNothingToUpdate) {
			return nil, err
		}
		s.logger.Error("failed to update task",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("task_id", taskID.String()))
		return nil, fmt.Errorf("update task: %w", err)
	}

	if statusChanged {
		from := current.Status
		s.recordStatusChange(ctx, taskID, userID, &from, updated.Status, now)
	}

	return updated, nil
}

func (s *TaskService) UpdateTaskStatus(ctx context.Context, taskID string, userID uuid.UUID, status models.TaskStatus, reviewer *string) error {
	id, err := uuid.Parse(strings.TrimSpace(taskID))
	if err != nil {
		return apperrors.ErrTaskNotFound
	}

	patch := models.TaskPatch{Status: &status}
	if reviewer != nil {
		patch.Reviewer = &reviewer
	}

	_, err = s.UpdateTask(ctx, userID, id, patch)
	return err
}

func (s *TaskService) MoveTask(ctx context.Context, userID, taskID uuid.UUID, status models.TaskStatus, afterID, beforeID *uuid.UUID) (*models.Task, error) {
	if !status.IsValid() {
		return nil, apperrors.ErrInvalidTaskStatus
	}

	current, err := s.GetTask(ctx, userID, taskID)
	if err != nil {
		return nil, err
	}

	position, err := s.taskRepo.PositionBetween(ctx, userID, status, afterID, beforeID)
	if err != nil {
		s.logger.Error("failed to compute position between neighbours",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("task_id", taskID.String()))
		return nil, fmt.Errorf("position between: %w", err)
	}

	patch := models.TaskPatch{Status: &status, Position: &position}
	if err := normalizeTaskPatch(&patch, current); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	statusChanged := applyTransitionTimestamps(&patch, current, now)

	updated, err := s.taskRepo.UpdateTask(ctx, userID, taskID, patch)
	if err != nil {
		if errors.Is(err, apperrors.ErrTaskNotFound) {
			return nil, err
		}
		s.logger.Error("failed to move task",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("task_id", taskID.String()))
		return nil, fmt.Errorf("move task: %w", err)
	}

	if statusChanged {
		from := current.Status
		s.recordStatusChange(ctx, taskID, userID, &from, updated.Status, now)
	}

	return updated, nil
}

func (s *TaskService) DeleteTask(ctx context.Context, userID, taskID uuid.UUID) error {
	if err := s.taskRepo.DeleteTask(ctx, userID, taskID); err != nil {
		if errors.Is(err, apperrors.ErrTaskNotFound) {
			return err
		}
		s.logger.Error("failed to delete task",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("task_id", taskID.String()))
		return fmt.Errorf("delete task: %w", err)
	}
	return nil
}

func (s *TaskService) CreateTasksFromPlan(ctx context.Context, userID uuid.UUID, sprintID *uuid.UUID, plan *models.SprintPlan) ([]models.Task, error) {
	if plan == nil || len(plan.Weeks) == 0 {
		return nil, apperrors.ErrNoPlanningItems
	}

	if err := s.assertSprintOwned(ctx, userID, sprintID); err != nil {
		return nil, err
	}

	position, err := s.taskRepo.AppendPosition(ctx, userID, models.StatusTodo)
	if err != nil {
		s.logger.Error("failed to compute append position", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("append position: %w", err)
	}

	now := time.Now().UTC()
	weeks := slices.Clone(plan.Weeks)
	slices.SortStableFunc(weeks, func(a, b models.PlannedWeek) int { return cmp.Compare(a.Index, b.Index) })

	tasks := make([]models.Task, 0, len(weeks))
	for _, week := range weeks {
		due := parsePlanDueDate(week.EndsOn)

		planned := slices.Clone(week.Tasks)
		slices.SortStableFunc(planned, func(a, b models.PlannedTask) int { return cmp.Compare(a.Order, b.Order) })

		for _, item := range planned {
			title := normalizeTaskText(&item.Title, maxTitleRunes)
			if title == nil {
				continue
			}

			priority := item.Priority
			if !priority.IsValid() {
				priority = models.PriorityMedium
			}

			tasks = append(tasks, models.Task{
				ID:            uuid.New(),
				UserID:        userID,
				Title:         *title,
				Description:   plannedDescription(item),
				Status:        models.StatusTodo,
				Priority:      priority,
				Reviewer:      normalizeTaskText(&item.Reviewer, maxReviewerRunes),
				EstimateHours: plannedHours(item.EstimateHours),
				BufferHours:   plannedHours(item.BufferHours),
				Blockers:      normalizeTaskText(&item.Blockers, 0),
				SprintID:      sprintID,
				DueDate:       due,
				Position:      position,
				CreatedAt:     now,
				UpdatedAt:     now,
			})
			position += planPositionStep
		}
	}

	if len(tasks) == 0 {
		return nil, apperrors.ErrNoPlanningItems
	}

	if err := s.taskRepo.CreateTasksBulk(ctx, tasks); err != nil {
		s.logger.Error("failed to create tasks from plan",
			zap.Error(err), zap.String("user_id", userID.String()), zap.Int("count", len(tasks)))
		return nil, fmt.Errorf("create tasks bulk: %w", err)
	}

	for i := range tasks {
		s.recordStatusChange(ctx, tasks[i].ID, userID, nil, tasks[i].Status, now)
	}

	return tasks, nil
}

// recordStatusChange appends one history row. History is an audit trail, not
// part of the write the caller asked for, so a failure is logged and swallowed.
func (s *TaskService) recordStatusChange(ctx context.Context, taskID, userID uuid.UUID, from *models.TaskStatus, to models.TaskStatus, at time.Time) {
	change := &models.StatusChange{
		ID:         uuid.New(),
		TaskID:     taskID,
		UserID:     userID,
		FromStatus: from,
		ToStatus:   to,
		ChangedAt:  at,
	}
	if err := s.taskRepo.RecordStatusChange(ctx, change); err != nil {
		s.logger.Warn("failed to record task status change",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("task_id", taskID.String()))
	}
}

// normalizeTaskPatch validates and canonicalises every field the patch actually
// supplies, rewriting it in place. current is the task being updated, or nil
// when the patch describes a task that does not exist yet; it supplies the
// fallback values the IN_REVIEW rule checks against.
func normalizeTaskPatch(patch *models.TaskPatch, current *models.Task) error {
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" {
			return apperrors.ErrEmptyTaskTitle
		}
		if utf8.RuneCountInString(title) > maxTitleRunes {
			return fmt.Errorf("%w: at most %d characters", apperrors.ErrTaskTitleTooLong, maxTitleRunes)
		}
		*patch.Title = title
	}

	if patch.Description != nil {
		*patch.Description = normalizeTaskText(*patch.Description, 0)
	}
	if patch.Blockers != nil {
		*patch.Blockers = normalizeTaskText(*patch.Blockers, 0)
	}
	if patch.Reviewer != nil {
		*patch.Reviewer = normalizeTaskText(*patch.Reviewer, maxReviewerRunes)
	}

	if patch.Status != nil && !patch.Status.IsValid() {
		return apperrors.ErrInvalidTaskStatus
	}
	if patch.Priority != nil && !patch.Priority.IsValid() {
		return apperrors.ErrInvalidTaskPriority
	}

	for _, hours := range []**float64{patch.EstimateHours, patch.BufferHours, patch.SpentHours} {
		if hours == nil {
			continue
		}
		rounded, err := normalizeTaskHours(*hours)
		if err != nil {
			return err
		}
		*hours = rounded
	}

	var status models.TaskStatus
	switch {
	case patch.Status != nil:
		status = *patch.Status
	case current != nil:
		status = current.Status
	}
	if status != models.StatusInReview {
		return nil
	}

	var reviewer *string
	if current != nil {
		reviewer = current.Reviewer
	}
	if patch.Reviewer != nil {
		reviewer = *patch.Reviewer
	}
	if reviewer == nil || strings.TrimSpace(*reviewer) == "" {
		return apperrors.ErrReviewerRequired
	}

	return nil
}

// normalizeTaskText trims v, collapsing an empty result to NULL. maxRunes of 0
// means the column is unbounded.
func normalizeTaskText(v *string, maxRunes int) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	if maxRunes > 0 && utf8.RuneCountInString(trimmed) > maxRunes {
		trimmed = string([]rune(trimmed)[:maxRunes])
	}
	return &trimmed
}

func normalizeTaskHours(v *float64) (*float64, error) {
	if v == nil {
		return nil, nil
	}
	if *v < 0 || *v > maxTaskHours {
		return nil, apperrors.ErrInvalidTaskHours
	}
	rounded := math.Round(*v*100) / 100
	return &rounded, nil
}

// applyTransitionTimestamps folds the started_at/completed_at side effects of a
// status change into the same patch, so one UPDATE carries the whole
// transition. It reports whether the status actually changed.
func applyTransitionTimestamps(patch *models.TaskPatch, current *models.Task, now time.Time) bool {
	if patch.Status == nil || current == nil || *patch.Status == current.Status {
		return false
	}
	next := *patch.Status

	if startsWork(next) && current.StartedAt == nil && patch.StartedAt == nil {
		started := &now
		patch.StartedAt = &started
	}

	switch {
	case next == models.StatusDone:
		completed := &now
		patch.CompletedAt = &completed
	case current.Status == models.StatusDone:
		var cleared *time.Time
		patch.CompletedAt = &cleared
	}

	return true
}

// startsWork reports whether entering status means the work is underway.
func startsWork(status models.TaskStatus) bool {
	switch status {
	case models.StatusInProgress, models.StatusInReview, models.StatusDone:
		return true
	}
	return false
}

func plannedHours(v float64) *float64 {
	if v <= 0 {
		return nil
	}
	rounded := math.Round(v*100) / 100
	if rounded > maxTaskHours {
		rounded = maxTaskHours
	}
	return &rounded
}

// plannedDescription renders the planned notes plus, when the item was broken
// down, a markdown checklist of its subtasks.
func plannedDescription(item models.PlannedTask) *string {
	description := strings.TrimSpace(item.Description)

	lines := make([]string, 0, len(item.Subtasks))
	for _, sub := range item.Subtasks {
		title := strings.TrimSpace(sub.Title)
		if title == "" {
			continue
		}
		lines = append(lines, "- [ ] "+title+" ("+strconv.FormatFloat(sub.EstimateHours, 'f', -1, 64)+"h)")
	}

	if len(lines) > 0 {
		checklist := strings.Join(lines, "\n")
		if description == "" {
			description = checklist
		} else {
			description += "\n\n" + checklist
		}
	}

	return normalizeTaskText(&description, 0)
}

func parsePlanDueDate(endsOn string) *time.Time {
	day, err := time.Parse(planDateLayout, strings.TrimSpace(endsOn))
	if err != nil {
		return nil
	}
	due := time.Date(day.Year(), day.Month(), day.Day(), 23, 59, 59, 0, time.UTC)
	return &due
}
