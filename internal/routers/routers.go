package routers

import (
	"net/http"
	"todo-app/internal/handler"
	"todo-app/internal/middleware"
	"todo-app/pkg/jwt"

	"github.com/gin-gonic/gin"
)

func SetupRoutes(
	r *gin.Engine,
	tokenManager *jwt.TokenManager,
	authHandler *handler.AuthHandler,
	taskHandler *handler.TaskHandler,
) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/google", authHandler.GoogleLogin)
		authGroup.POST("/refresh", authHandler.RefreshToken)
	}

	apiGroup := r.Group("/api")
	apiGroup.Use(middleware.AuthMiddleware(tokenManager))
	{
		apiGroup.POST("/tasks", taskHandler.CreateTask)
		apiGroup.GET("/tasks", taskHandler.GetTasks)
		apiGroup.PATCH("/tasks/:id/status", taskHandler.UpdateTaskStatus)
		apiGroup.POST("/tasks/share", taskHandler.ShareTasks)
	}
}
