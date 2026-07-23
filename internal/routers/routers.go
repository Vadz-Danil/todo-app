package routers

import (
	"net/http"
	"time"
	"todo-app/internal/handler"
	"todo-app/internal/middleware"
	"todo-app/pkg/jwt"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func SetupRoutes(
	r *gin.Engine,
	tokenManager *jwt.TokenManager,
	authHandler *handler.AuthHandler,
	taskHandler *handler.TaskHandler,
) {
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:5173", "http://localhost:3000", "https://harmonious-prosperity-production-b588.up.railway.app"},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

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
