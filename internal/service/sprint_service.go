package service

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	maxSprintNameLen       = 255
	maxSprintCapacityHours = 10000
	maxSprintSpan          = 366 * 24 * time.Hour
)

var _ Sprint = (*SprintService)(nil)

type SprintService struct {
	sprintRepo repository.SprintRepository
	taskRepo   repository.TaskRepository
	logger     *zap.Logger
}

func NewSprintService(sprintRepo repository.SprintRepository, taskRepo repository.TaskRepository, logger *zap.Logger) *SprintService {
	return &SprintService{
		sprintRepo: sprintRepo,
		taskRepo:   taskRepo,
		logger:     logger,
	}
}

func (s *SprintService) Create(ctx context.Context, userID uuid.UUID, in SprintInput) (*models.Sprint, error) {
	in, err := s.validate(in)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	sprint := &models.Sprint{
		ID:            uuid.New(),
		UserID:        userID,
		Name:          in.Name,
		Goal:          in.Goal,
		StartsOn:      in.StartsOn,
		EndsOn:        in.EndsOn,
		CapacityHours: in.CapacityHours,
		Status:        in.Status,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.sprintRepo.CreateSprint(ctx, sprint); err != nil {
		s.logger.Error("failed to create sprint", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to create sprint: %w", err)
	}

	return sprint, nil
}

func (s *SprintService) Get(ctx context.Context, userID, sprintID uuid.UUID) (*models.Sprint, error) {
	sprint, err := s.load(ctx, userID, sprintID)
	if err != nil {
		return nil, err
	}
	return s.withTasks(ctx, userID, sprint)
}

func (s *SprintService) List(ctx context.Context, userID uuid.UUID) ([]models.Sprint, error) {
	sprints, err := s.sprintRepo.ListSprints(ctx, userID)
	if err != nil {
		s.logger.Error("failed to list sprints", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to list sprints: %w", err)
	}
	if len(sprints) == 0 {
		return sprints, nil
	}

	tasks, err := s.taskRepo.ListTasks(ctx, userID, models.TaskFilter{})
	if err != nil {
		s.logger.Error("failed to list tasks for sprint stats", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("failed to list tasks for sprint stats: %w", err)
	}

	tallies := make(map[uuid.UUID]*sprintTally, len(sprints))
	for i := range sprints {
		tallies[sprints[i].ID] = &sprintTally{}
	}
	for i := range tasks {
		if tasks[i].SprintID == nil {
			continue
		}
		if tally, ok := tallies[*tasks[i].SprintID]; ok {
			tally.add(tasks[i])
		}
	}

	for i := range sprints {
		sprints[i].Tasks = nil
		sprints[i].Stats = tallies[sprints[i].ID].stats(sprints[i].CapacityHours)
	}

	return sprints, nil
}

func (s *SprintService) Update(ctx context.Context, userID, sprintID uuid.UUID, in SprintInput) (*models.Sprint, error) {
	sprint, err := s.load(ctx, userID, sprintID)
	if err != nil {
		return nil, err
	}

	in, err = s.validate(in)
	if err != nil {
		return nil, err
	}

	sprint.Name = in.Name
	sprint.Goal = in.Goal
	sprint.StartsOn = in.StartsOn
	sprint.EndsOn = in.EndsOn
	sprint.CapacityHours = in.CapacityHours
	sprint.Status = in.Status
	sprint.UpdatedAt = time.Now().UTC()

	if err := s.sprintRepo.UpdateSprint(ctx, sprint); err != nil {
		if errors.Is(err, apperrors.ErrSprintNotFound) {
			return nil, err
		}
		s.logger.Error("failed to update sprint", zap.Error(err), zap.String("sprint_id", sprintID.String()))
		return nil, fmt.Errorf("failed to update sprint: %w", err)
	}

	return s.withTasks(ctx, userID, sprint)
}

// Delete only removes the sprint: the tasks.sprint_id foreign key is
// ON DELETE SET NULL, so the database detaches the tasks by itself.
func (s *SprintService) Delete(ctx context.Context, userID, sprintID uuid.UUID) error {
	if err := s.sprintRepo.DeleteSprint(ctx, userID, sprintID); err != nil {
		if errors.Is(err, apperrors.ErrSprintNotFound) {
			return err
		}
		s.logger.Error("failed to delete sprint", zap.Error(err), zap.String("sprint_id", sprintID.String()))
		return fmt.Errorf("failed to delete sprint: %w", err)
	}
	return nil
}

func (s *SprintService) load(ctx context.Context, userID, sprintID uuid.UUID) (*models.Sprint, error) {
	sprint, err := s.sprintRepo.GetSprintByID(ctx, userID, sprintID)
	if err != nil {
		if errors.Is(err, apperrors.ErrSprintNotFound) {
			return nil, err
		}
		s.logger.Error("failed to get sprint", zap.Error(err), zap.String("sprint_id", sprintID.String()))
		return nil, fmt.Errorf("failed to get sprint: %w", err)
	}
	return sprint, nil
}

func (s *SprintService) withTasks(ctx context.Context, userID uuid.UUID, sprint *models.Sprint) (*models.Sprint, error) {
	tasks, err := s.taskRepo.ListTasks(ctx, userID, models.TaskFilter{SprintID: &sprint.ID})
	if err != nil {
		s.logger.Error("failed to list sprint tasks", zap.Error(err), zap.String("sprint_id", sprint.ID.String()))
		return nil, fmt.Errorf("failed to list sprint tasks: %w", err)
	}

	var tally sprintTally
	for i := range tasks {
		tally.add(tasks[i])
	}

	sprint.Tasks = tasks
	sprint.Stats = tally.stats(sprint.CapacityHours)
	return sprint, nil
}

func (s *SprintService) validate(in SprintInput) (SprintInput, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > maxSprintNameLen {
		return SprintInput{}, apperrors.ErrInvalidSprint
	}

	if in.Goal != nil {
		goal := strings.TrimSpace(*in.Goal)
		if goal == "" {
			in.Goal = nil
		} else {
			in.Goal = &goal
		}
	}

	in.StartsOn = s.dateOnly(in.StartsOn)
	in.EndsOn = s.dateOnly(in.EndsOn)
	if in.EndsOn.Before(in.StartsOn) || in.EndsOn.Sub(in.StartsOn) > maxSprintSpan {
		return SprintInput{}, apperrors.ErrInvalidSprint
	}

	if in.CapacityHours != nil && (*in.CapacityHours < 0 || *in.CapacityHours > maxSprintCapacityHours) {
		return SprintInput{}, apperrors.ErrInvalidSprint
	}

	if in.Status == "" {
		in.Status = models.SprintPlanned
	}
	if !in.Status.IsValid() {
		return SprintInput{}, apperrors.ErrInvalidSprint
	}

	return in, nil
}

// dateOnly pins a timestamp to midnight UTC of its UTC calendar day so it
// round-trips through the DATE columns unchanged.
func (s *SprintService) dateOnly(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

// sprintTally folds a stream of tasks into the numbers behind models.SprintStats.
type sprintTally struct {
	total   int
	done    int
	planned float64
	buffer  float64
}

func (t *sprintTally) add(task models.Task) {
	t.total++
	if task.Status == models.StatusDone {
		t.done++
	}
	if task.EstimateHours != nil {
		t.planned += *task.EstimateHours
	}
	if task.BufferHours != nil {
		t.buffer += *task.BufferHours
	}
}

func (t *sprintTally) stats(capacityHours *float64) *models.SprintStats {
	committed := t.planned + t.buffer
	stats := &models.SprintStats{
		TotalTasks:     t.total,
		DoneTasks:      t.done,
		PlannedHours:   t.planned,
		BufferHours:    t.buffer,
		CommittedHours: committed,
	}

	if t.total > 0 {
		stats.CompletionRate = math.Round(float64(t.done)/float64(t.total)*10000) / 10000
	}
	if capacityHours != nil && *capacityHours > 0 {
		stats.LoadPercent = math.Round(committed / *capacityHours * 1000) / 10
	}

	return stats
}
