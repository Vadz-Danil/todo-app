package apperrors

import "errors"

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user with this email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrTaskNotFound       = errors.New("task not found or permission denied")
	ErrEmptyTaskTitle     = errors.New("task title cannot be empty")
	ErrInvalidTaskStatus  = errors.New("invalid task status")
	ErrEmptyRecipient     = errors.New("recipient email is required")
	ErrEmptyTaskList      = errors.New("task list is empty, nothing to share")
	ErrUnauthorized       = errors.New("unauthorized access")
)
