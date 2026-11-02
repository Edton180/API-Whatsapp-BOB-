package controllers

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

type InstanceSettings struct {
	RejectCall      bool   `json:"rejectCall"`
	MsgCall         string `json:"msgCall"`
	GroupsIgnore    bool   `json:"groupsIgnore"`
	AlwaysOnline    bool   `json:"alwaysOnline"`
	ReadMessages    bool   `json:"readMessages"`
	ReadStatus      bool   `json:"readStatus"`
	SyncFullHistory bool   `json:"syncFullHistory"`
}

var (
	settingsMu    sync.RWMutex
	settingsStore = make(map[string]InstanceSettings)
)

func SetSettings(c *gin.Context) {
	instance := c.Param("instance")
	var req InstanceSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	settingsMu.Lock()
	settingsStore[instance] = req
	settingsMu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"instance": instance,
		"settings": req,
	})
}

func FindSettings(c *gin.Context) {
	instance := c.Param("instance")
	settingsMu.RLock()
	cfg, exists := settingsStore[instance]
	settingsMu.RUnlock()
	if !exists {
		cfg = InstanceSettings{AlwaysOnline: true}
	}
	c.JSON(http.StatusOK, gin.H{
		"instance": instance,
		"settings": cfg,
	})
}
