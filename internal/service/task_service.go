package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"todo-app/internal/apperrors"
	"todo-app/internal/models"
	"todo-app/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type TaskService struct {
	taskRepo repository.TaskRepository
	logger   *zap.Logger
}

func NewTaskService(taskRepo repository.TaskRepository, logger *zap.Logger) *TaskService {
	return &TaskService{
		taskRepo: taskRepo,
		logger:   logger,
	}
}

func (s *TaskService) CreateTask(ctx context.Context, userID uuid.UUID, title string, description *string) (*models.Task, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, apperrors.ErrEmptyTaskTitle
	}

	if description != nil {
		trimmed := strings.TrimSpace(*description)
		if trimmed == "" {
			description = nil
		} else {
			description = &trimmed
		}
	}

	task := &models.Task{
		ID:          uuid.New(),
		UserID:      userID,
		Title:       title,
		Description: description,
		Status:      models.StatusTodo,
		CreatedAt:   time.Now(),
	}

	if err := s.taskRepo.CreateTask(ctx, task); err != nil {
		s.logger.Error("failed to create task", zap.Error(err))
		return nil, err
	}

	return task, nil
}

func (s *TaskService) GetTasks(ctx context.Context, userID uuid.UUID) ([]models.Task, error) {
	tasks, err := s.taskRepo.GetTasksByUserID(ctx, userID)
	if err != nil {
		s.logger.Error("failed to get tasks", zap.Error(err))
		return nil, err
	}

	return tasks, nil
}

func (s *TaskService) UpdateTaskStatus(ctx context.Context, taskID string, userID uuid.UUID, status models.TaskStatus) error {
	if status != models.StatusTodo && status != models.StatusDone {
		return apperrors.ErrInvalidTaskStatus
	}

	err := s.taskRepo.UpdateTaskStatus(ctx, taskID, userID, status)
	if err != nil {
		if errors.Is(err, apperrors.ErrTaskNotFound) {
			return apperrors.ErrTaskNotFound
		}
		s.logger.Error("failed to update task status", zap.Error(err))
		return err
	}

	return nil
}
