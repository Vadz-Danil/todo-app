package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const maxSubtaskTitleRunes = 500

// SubtaskTaskGuard is the one thing the subtask service needs from the task
// side: proof that a task belongs to the caller before a checklist is attached
// to it. Narrowing to a single method keeps checklists away from task writes.
type SubtaskTaskGuard interface {
	GetTask(ctx context.Context, userID, taskID uuid.UUID) (*models.Task, error)
}

type SubtaskService struct {
	repo   repository.SubtaskRepository
	tasks  SubtaskTaskGuard
	logger *zap.Logger
}

func NewSubtaskService(repo repository.SubtaskRepository, tasks SubtaskTaskGuard, logger *zap.Logger) *SubtaskService {
	return &SubtaskService{repo: repo, tasks: tasks, logger: logger}
}

// List returns a task's checklist, after confirming the caller owns the task.
func (s *SubtaskService) List(ctx context.Context, userID, taskID uuid.UUID) ([]models.Subtask, error) {
	if err := s.assertTaskOwned(ctx, userID, taskID); err != nil {
		return nil, err
	}

	items, err := s.repo.ListByTask(ctx, userID, taskID)
	if err != nil {
		s.logger.Error("failed to list subtasks", zap.Error(err), zap.String("task_id", taskID.String()))
		return nil, fmt.Errorf("list subtasks: %w", err)
	}
	return items, nil
}

// Create appends a checklist item to a task the caller owns.
func (s *SubtaskService) Create(ctx context.Context, userID, taskID uuid.UUID, in models.SubtaskCreate) (*models.Subtask, error) {
	title, err := normalizeSubtaskTitle(in.Title)
	if err != nil {
		return nil, err
	}

	if err := s.assertTaskOwned(ctx, userID, taskID); err != nil {
		return nil, err
	}

	position, err := s.repo.NextPosition(ctx, userID, taskID)
	if err != nil {
		s.logger.Error("failed to compute subtask position", zap.Error(err), zap.String("task_id", taskID.String()))
		return nil, fmt.Errorf("subtask position: %w", err)
	}

	item := &models.Subtask{
		ID:       uuid.New(),
		TaskID:   taskID,
		UserID:   userID,
		Title:    title,
		Position: position,
	}
	if err := s.repo.Create(ctx, item); err != nil {
		if errors.Is(err, apperrors.ErrTaskNotFound) || errors.Is(err, apperrors.ErrEmptySubtaskTitle) {
			return nil, err
		}
		s.logger.Error("failed to create subtask", zap.Error(err), zap.String("task_id", taskID.String()))
		return nil, fmt.Errorf("create subtask: %w", err)
	}

	return item, nil
}

// Update edits a checklist item's title or done flag. Ownership is enforced by
// the repository's user_id predicate, so no separate task lookup is needed.
func (s *SubtaskService) Update(ctx context.Context, userID, subtaskID uuid.UUID, patch models.SubtaskPatch) (*models.Subtask, error) {
	if patch.Title != nil {
		title, err := normalizeSubtaskTitle(*patch.Title)
		if err != nil {
			return nil, err
		}
		patch.Title = &title
	}

	item, err := s.repo.Update(ctx, userID, subtaskID, patch)
	if err != nil {
		if errors.Is(err, apperrors.ErrSubtaskNotFound) ||
			errors.Is(err, apperrors.ErrNothingToUpdate) ||
			errors.Is(err, apperrors.ErrEmptySubtaskTitle) {
			return nil, err
		}
		s.logger.Error("failed to update subtask", zap.Error(err), zap.String("subtask_id", subtaskID.String()))
		return nil, fmt.Errorf("update subtask: %w", err)
	}

	return item, nil
}

func (s *SubtaskService) Delete(ctx context.Context, userID, subtaskID uuid.UUID) error {
	if err := s.repo.Delete(ctx, userID, subtaskID); err != nil {
		if errors.Is(err, apperrors.ErrSubtaskNotFound) {
			return err
		}
		s.logger.Error("failed to delete subtask", zap.Error(err), zap.String("subtask_id", subtaskID.String()))
		return fmt.Errorf("delete subtask: %w", err)
	}
	return nil
}

// Progress returns the done/total rollup for the given tasks.
func (s *SubtaskService) Progress(ctx context.Context, userID uuid.UUID, taskIDs []uuid.UUID) (map[uuid.UUID]models.SubtaskProgress, error) {
	progress, err := s.repo.ProgressByTasks(ctx, userID, taskIDs)
	if err != nil {
		s.logger.Error("failed to load subtask progress", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("subtask progress: %w", err)
	}
	return progress, nil
}

// assertTaskOwned turns a foreign task into ErrTaskNotFound, which handlers map
// to 404 — the same answer a missing task gives, so ownership cannot be probed.
func (s *SubtaskService) assertTaskOwned(ctx context.Context, userID, taskID uuid.UUID) error {
	if _, err := s.tasks.GetTask(ctx, userID, taskID); err != nil {
		return err
	}
	return nil
}

func normalizeSubtaskTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", apperrors.ErrEmptySubtaskTitle
	}
	if utf8.RuneCountInString(title) > maxSubtaskTitleRunes {
		return "", apperrors.ErrSubtaskTitleTooLong
	}
	return title, nil
}
