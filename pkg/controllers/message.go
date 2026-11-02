package controllers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type TextPayload struct {
	Number           string `json:"number" binding:"required"`
	Text             string `json:"text" binding:"required"`
	Delay            int    `json:"delay"`
	LinkPreview      bool   `json:"linkPreview"`
	MentionsEveryOne bool   `json:"mentionsEveryOne"`
}

type MediaPayload struct {
	Number    string `json:"number" binding:"required"`
	Mediatype string `json:"mediatype" binding:"required"`
	Mimetype  string `json:"mimetype"`
	Caption   string `json:"caption"`
	Media     string `json:"media" binding:"required"`
	FileName  string `json:"fileName"`
	Delay     int    `json:"delay"`
}

type AudioPTTPayload struct {
	Number   string `json:"number" binding:"required"`
	Audio    string `json:"audio" binding:"required"`
	Delay    int    `json:"delay"`
	Encoding bool   `json:"encoding"`
}

type PollPayload struct {
	Number          string   `json:"number" binding:"required"`
	Name            string   `json:"name" binding:"required"`
	SelectableCount int      `json:"selectableCount"`
	Values          []string `json:"values" binding:"required"`
}

type ListPayload struct {
	Number      string        `json:"number" binding:"required"`
	Title       string        `json:"title" binding:"required"`
	Description string        `json:"description"`
	ButtonText  string        `json:"buttonText" binding:"required"`
	FooterText  string        `json:"footerText"`
	Sections    []interface{} `json:"sections" binding:"required"`
}

type ButtonsPayload struct {
	Number      string        `json:"number" binding:"required"`
	Title       string        `json:"title"`
	Description string        `json:"description" binding:"required"`
	Footer      string        `json:"footer"`
	Buttons     []interface{} `json:"buttons" binding:"required"`
}

type LocationPayload struct {
	Number    string  `json:"number" binding:"required"`
	Name      string  `json:"name"`
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude" binding:"required"`
	Longitude float64 `json:"longitude" binding:"required"`
}

type ReactionPayload struct {
	Key      map[string]interface{} `json:"key" binding:"required"`
	Reaction string                 `json:"reaction" binding:"required"`
}

func SendText(c *gin.Context) {
	instance := c.Param("instance")
	var req TextPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"key": map[string]interface{}{
			"remoteJid": req.Number + "@s.whatsapp.net",
			"fromMe":    true,
			"id":        "BOB_" + time.Now().Format("20060102150405"),
		},
		"message":          map[string]string{"conversation": req.Text},
		"messageTimestamp": time.Now().Unix(),
		"status":           "PENDING",
		"instance":         instance,
	})
}

func SendMedia(c *gin.Context) {
	instance := c.Param("instance")
	var req MediaPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"instance":  instance,
		"mediatype": req.Mediatype,
		"caption":   req.Caption,
		"status":    "SENT",
		"timestamp": time.Now().Unix(),
	})
}

func SendWhatsAppAudio(c *gin.Context) {
	instance := c.Param("instance")
	var req AudioPTTPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"instance": instance,
		"ptt":      true,
		"status":   "SENT",
	})
}

func SendPoll(c *gin.Context) {
	instance := c.Param("instance")
	var req PollPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"instance": instance, "poll": req.Name, "options": req.Values, "status": "SENT"})
}

func SendList(c *gin.Context) {
	instance := c.Param("instance")
	var req ListPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"instance": instance, "listTitle": req.Title, "status": "SENT"})
}

func SendButtons(c *gin.Context) {
	instance := c.Param("instance")
	var req ButtonsPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"instance": instance, "buttons": len(req.Buttons), "status": "SENT"})
}

func SendLocation(c *gin.Context) {
	instance := c.Param("instance")
	var req LocationPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"instance": instance, "lat": req.Latitude, "lng": req.Longitude, "status": "SENT"})
}

func SendReaction(c *gin.Context) {
	instance := c.Param("instance")
	var req ReactionPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"instance": instance, "reaction": req.Reaction, "status": "SENT"})
}

func SendSticker(c *gin.Context) {
	c.JSON(http.StatusCreated, gin.H{"instance": c.Param("instance"), "type": "sticker", "status": "SENT"})
}

func SendContact(c *gin.Context) {
	c.JSON(http.StatusCreated, gin.H{"instance": c.Param("instance"), "type": "vcard", "status": "SENT"})
}

func SendStatus(c *gin.Context) {
	c.JSON(http.StatusCreated, gin.H{"instance": c.Param("instance"), "type": "status_broadcast", "status": "SENT"})
}
