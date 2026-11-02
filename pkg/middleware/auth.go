package middleware

import (
	"net/http"
	"strings"

	"github.com/Edton180/API-Whatsapp-BOB-/pkg/config"
	"github.com/gin-gonic/gin"
)

func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, apikey, Authorization, X-Tenant-ID")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		if c.Request.URL.Path == "/" || c.Request.URL.Path == "/health" || c.Request.URL.Path == "/donate/pix" {
			c.Next()
			return
		}

		expectedKey := ""
		if config.Env != nil {
			expectedKey = config.Env.GlobalAPIKey
		}
		providedKey := c.GetHeader("apikey")
		if providedKey == "" {
			providedKey = strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		}

		if expectedKey != "" && providedKey != expectedKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"status":  401,
				"error":   "Unauthorized",
				"message": "GLOBAL_API_KEY inválida ou ausente.",
			})
			return
		}

		tenantID := c.GetHeader("X-Tenant-ID")
		if tenantID == "" {
			tenantID = "default"
		}
		c.Set("tenant_id", tenantID)
		c.Next()
	}
}
