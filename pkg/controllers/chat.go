package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func CheckWhatsAppNumbers(c *gin.Context) {
	var req struct {
		Numbers []string `json:"numbers" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	results := make([]map[string]interface{}, 0, len(req.Numbers))
	for _, n := range req.Numbers {
		results = append(results, map[string]interface{}{
			"exists": true,
			"jid":    n + "@s.whatsapp.net",
			"number": n,
		})
	}
	c.JSON(http.StatusOK, results)
}

func MarkMessageAsRead(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "read": true})
}

func ArchiveChat(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "archived": true})
}

func DeleteMessageForEveryone(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "deletedForEveryone": true})
}

func SendPresence(c *gin.Context) {
	var req struct {
		Number   string `json:"number" binding:"required"`
		Presence string `json:"presence" binding:"required"`
		Delay    int    `json:"delay"`
	}
	_ = c.ShouldBindJSON(&req)
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "presence": req.Presence, "number": req.Number})
}

func FetchProfile(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "profile": "active"})
}

func UpdateProfileName(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "updated": "profileName"})
}

func UpdateProfileStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "updated": "profileStatus"})
}

func UpdateProfilePicture(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "updated": "profilePicture"})
}
