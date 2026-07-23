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

// Handlers bundles every HTTP handler so the route table stays readable as the
// surface grows.
type Handlers struct {
	Auth      *handler.AuthHandler
	Task      *handler.TaskHandler
	Analytics *handler.AnalyticsHandler
	AI        *handler.AIHandler
	Sprint    *handler.SprintHandler
	Export    *handler.ExportHandler
	Share     *handler.ShareHandler
}

func SetupRoutes(r *gin.Engine, tokenManager *jwt.TokenManager, h Handlers, allowedOrigins []string) {
	r.Use(cors.New(cors.Config{
		AllowOrigins:     allowedOrigins,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		ExposeHeaders:    []string{"Content-Length", "Content-Disposition"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Public, unauthenticated: the token in the path is the credential.
	r.GET("/public/share/:token", h.Share.Resolve)

	authGroup := r.Group("/auth")
	{
		authGroup.POST("/register", h.Auth.Register)
		authGroup.POST("/login", h.Auth.Login)
		authGroup.POST("/google", h.Auth.GoogleLogin)
		authGroup.POST("/refresh", h.Auth.RefreshToken)
	}

	apiGroup := r.Group("/api")
	apiGroup.Use(middleware.AuthMiddleware(tokenManager))
	{
		apiGroup.GET("/me", h.Auth.Me)

		tasks := apiGroup.Group("/tasks")
		{
			tasks.GET("", h.Task.GetTasks)
			tasks.POST("", h.Task.CreateTask)
			tasks.POST("/share", h.Task.ShareTasks)
			tasks.GET("/:id", h.Task.GetTask)
			tasks.PATCH("/:id", h.Task.UpdateTask)
			tasks.DELETE("/:id", h.Task.DeleteTask)
			tasks.PATCH("/:id/status", h.Task.UpdateTaskStatus)
			tasks.PATCH("/:id/move", h.Task.MoveTask)
		}

		shares := apiGroup.Group("/share-links")
		{
			shares.GET("", h.Share.ListLinks)
			shares.POST("", h.Share.CreateLink)
			shares.POST("/:id/revoke", h.Share.RevokeLink)
			shares.DELETE("/:id", h.Share.DeleteLink)
		}

		analytics := apiGroup.Group("/analytics")
		{
			analytics.GET("/dashboard", h.Analytics.Dashboard)
			analytics.GET("/export", h.Analytics.Export)
		}

		ai := apiGroup.Group("/ai")
		{
			ai.GET("/status", h.AI.Status)
			ai.POST("/summary", h.AI.Summary)
			ai.GET("/summaries", h.AI.ListSummaries)

			planning := ai.Group("/planning/sessions")
			{
				planning.POST("", h.AI.StartPlanning)
				planning.GET("", h.AI.ListSessions)
				planning.GET("/:id", h.AI.GetSession)
				planning.DELETE("/:id", h.AI.DeleteSession)
				planning.POST("/:id/answers", h.AI.AnswerPlanning)
				planning.POST("/:id/plan", h.AI.GeneratePlan)
				planning.POST("/:id/commit", h.AI.CommitPlan)
			}
		}

		sprints := apiGroup.Group("/sprints")
		{
			sprints.GET("", h.Sprint.List)
			sprints.POST("", h.Sprint.Create)
			sprints.GET("/:id", h.Sprint.Get)
			sprints.PATCH("/:id", h.Sprint.Update)
			sprints.DELETE("/:id", h.Sprint.Delete)
		}

		export := apiGroup.Group("/export")
		{
			export.GET("/targets", h.Export.ListTargets)
			export.POST("/targets", h.Export.CreateTarget)
			export.PATCH("/targets/:id", h.Export.UpdateTarget)
			export.DELETE("/targets/:id", h.Export.DeleteTarget)
			export.GET("/preview", h.Export.Preview)
			export.POST("/push", h.Export.Push)
			export.GET("/deliveries", h.Export.ListDeliveries)
		}
	}
}
