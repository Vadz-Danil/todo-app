package handler

import "todo-app/internal/models"

type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type GoogleLoginRequest struct {
	Code string `json:"code" binding:"required"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type CreateTaskRequest struct {
	Title       string  `json:"title" binding:"required"`
	Description *string `json:"description"`
}

type UpdateTaskStatusRequest struct {
	Status models.TaskStatus `json:"status" binding:"required"`
}

type ShareTasksRequest struct {
	RecipientEmail string `json:"recipient_email" binding:"required,email"`
}
