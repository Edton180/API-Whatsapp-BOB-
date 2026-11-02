package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func CreateGroup(c *gin.Context) {
	var req struct {
		Subject      string   `json:"subject" binding:"required"`
		Description  string   `json:"description"`
		Participants []string `json:"participants" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"instance":     c.Param("instance"),
		"subject":      req.Subject,
		"participants": req.Participants,
		"status":       "CREATED",
	})
}

func FetchAllGroups(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"instance": c.Param("instance"),
		"groups":   []interface{}{},
	})
}

func FindGroupInfos(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"instance": c.Param("instance"),
		"groupJid": c.Query("groupJid"),
	})
}

func UpdateGroupParticipants(c *gin.Context) {
	var req struct {
		Action       string   `json:"action" binding:"required"`
		Participants []string `json:"participants" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"instance":     c.Param("instance"),
		"action":       req.Action,
		"participants": req.Participants,
	})
}

func UpdateGroupSubject(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "updated": "subject"})
}

func UpdateGroupDescription(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "updated": "description"})
}

func UpdateGroupSetting(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "updated": "setting"})
}

func FetchInviteCode(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "inviteUrl": "https://chat.whatsapp.com/BOB_INVITE"})
}

func RevokeInviteCode(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "revoked": true})
}

func LeaveGroup(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"instance": c.Param("instance"), "left": true})
}
