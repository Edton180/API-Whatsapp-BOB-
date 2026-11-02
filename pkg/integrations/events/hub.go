package events

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type IntegrationStore struct {
	mu      sync.RWMutex
	configs map[string]map[string]interface{}
}

var Hub = &IntegrationStore{
	configs: make(map[string]map[string]interface{}),
}

func ConfigureIntegration(integrationType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		instance := c.Param("instance")
		var payload map[string]interface{}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		Hub.mu.Lock()
		if Hub.configs[instance] == nil {
			Hub.configs[instance] = make(map[string]interface{})
		}
		Hub.configs[instance][integrationType] = payload
		Hub.mu.Unlock()

		c.JSON(http.StatusOK, gin.H{
			"status":      "ENABLED",
			"instance":    instance,
			"integration": integrationType,
			"config":      payload,
		})
	}
}

func FindIntegration(integrationType string) gin.HandlerFunc {
	return func(c *gin.Context) {
		instance := c.Param("instance")
		Hub.mu.RLock()
		cfg := Hub.configs[instance][integrationType]
		Hub.mu.RUnlock()

		c.JSON(http.StatusOK, gin.H{
			"instance":    instance,
			"integration": integrationType,
			"config":      cfg,
		})
	}
}
