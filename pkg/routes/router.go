package routes

import (
	"net/http"

	"github.com/Edton180/API-Whatsapp-BOB-/pkg/controllers"
	"github.com/Edton180/API-Whatsapp-BOB-/pkg/core"
	"github.com/Edton180/API-Whatsapp-BOB-/pkg/integrations/events"
	"github.com/gin-gonic/gin"
)

const OfficialPixPayload = "00020126360014br.gov.bcb.pix0111046469472855204000053039865802BR5925EDTON KAIK BRAZ DA CONCEI6007TUCURUI62070503***63046E07"

func RegisterAllEvolutionAndBobRoutes(r *gin.Engine) {
	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, core.Info)
	})
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, core.Info)
	})

	r.GET("/donate/pix", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"project":        "API WhatsApp BOB",
			"creator":        "Edton Kaik Braz Da Conceição",
			"pix_key_cpf":    "04646947285",
			"formatted_cpf":  "046.469.472-85",
			"city":           "TUCURUI - PA",
			"emv_copia_cola": OfficialPixPayload,
		})
	})

	settings := r.Group("/settings")
	{
		settings.POST("/set/:instance", controllers.SetSettings)
		settings.GET("/find/:instance", controllers.FindSettings)
	}

	msg := r.Group("/message")
	{
		msg.POST("/sendText/:instance", controllers.SendText)
		msg.POST("/sendMedia/:instance", controllers.SendMedia)
		msg.POST("/sendWhatsAppAudio/:instance", controllers.SendWhatsAppAudio)
		msg.POST("/sendPoll/:instance", controllers.SendPoll)
		msg.POST("/sendList/:instance", controllers.SendList)
		msg.POST("/sendButtons/:instance", controllers.SendButtons)
		msg.POST("/sendLocation/:instance", controllers.SendLocation)
		msg.POST("/sendReaction/:instance", controllers.SendReaction)
		msg.POST("/sendSticker/:instance", controllers.SendSticker)
		msg.POST("/sendContact/:instance", controllers.SendContact)
		msg.POST("/sendStatus/:instance", controllers.SendStatus)
	}

	grp := r.Group("/group")
	{
		grp.POST("/create/:instance", controllers.CreateGroup)
		grp.GET("/fetchAllGroups/:instance", controllers.FetchAllGroups)
		grp.GET("/findGroupInfos/:instance", controllers.FindGroupInfos)
		grp.POST("/updateParticipant/:instance", controllers.UpdateGroupParticipants)
		grp.POST("/updateSubject/:instance", controllers.UpdateGroupSubject)
		grp.POST("/updateDescription/:instance", controllers.UpdateGroupDescription)
		grp.POST("/updateSetting/:instance", controllers.UpdateGroupSetting)
		grp.GET("/inviteCode/:instance", controllers.FetchInviteCode)
		grp.POST("/revokeInviteCode/:instance", controllers.RevokeInviteCode)
		grp.DELETE("/leaveGroup/:instance", controllers.LeaveGroup)
	}

	chat := r.Group("/chat")
	{
		chat.POST("/whatsappNumbers/:instance", controllers.CheckWhatsAppNumbers)
		chat.POST("/markMessageAsRead/:instance", controllers.MarkMessageAsRead)
		chat.POST("/archiveChat/:instance", controllers.ArchiveChat)
		chat.DELETE("/deleteMessageForEveryone/:instance", controllers.DeleteMessageForEveryone)
		chat.POST("/sendPresence/:instance", controllers.SendPresence)
		chat.POST("/fetchProfile/:instance", controllers.FetchProfile)
		chat.POST("/updateProfileName/:instance", controllers.UpdateProfileName)
		chat.POST("/updateProfileStatus/:instance", controllers.UpdateProfileStatus)
		chat.POST("/updateProfilePicture/:instance", controllers.UpdateProfilePicture)
	}

	r.POST("/chatwoot/set/:instance", events.ConfigureIntegration("chatwoot"))
	r.GET("/chatwoot/find/:instance", events.FindIntegration("chatwoot"))
	r.POST("/typebot/set/:instance", events.ConfigureIntegration("typebot"))
	r.GET("/typebot/find/:instance", events.FindIntegration("typebot"))
	r.POST("/n8n/set/:instance", events.ConfigureIntegration("n8n"))
	r.GET("/n8n/find/:instance", events.FindIntegration("n8n"))
	r.POST("/openai/set/:instance", events.ConfigureIntegration("openai"))
	r.GET("/openai/find/:instance", events.FindIntegration("openai"))
	r.POST("/dify/set/:instance", events.ConfigureIntegration("dify"))
	r.GET("/dify/find/:instance", events.FindIntegration("dify"))
	r.POST("/evolutionBot/set/:instance", events.ConfigureIntegration("evolutionBot"))
	r.GET("/evolutionBot/find/:instance", events.FindIntegration("evolutionBot"))
	r.POST("/webhook/set/:instance", events.ConfigureIntegration("webhook"))
	r.GET("/webhook/find/:instance", events.FindIntegration("webhook"))
	r.POST("/rabbitmq/set/:instance", events.ConfigureIntegration("rabbitmq"))
	r.POST("/sqs/set/:instance", events.ConfigureIntegration("sqs"))
	r.POST("/websocket/set/:instance", events.ConfigureIntegration("websocket"))
}
