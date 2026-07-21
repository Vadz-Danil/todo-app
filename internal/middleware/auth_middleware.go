package middleware

import (
	"net/http"
	"strings"
	"todo-app/internal/apperrors"
	"todo-app/pkg/jwt"

	"github.com/gin-gonic/gin"
)

const UserIDKey = "userID"

func AuthMiddleware(tokenManager *jwt.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing or invalid authorization header"})
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")

		userID, err := tokenManager.ParseAccessToken(tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": apperrors.ErrUnauthorized.Error()})
			return
		}

		c.Set(UserIDKey, userID)
		c.Next()
	}
}
