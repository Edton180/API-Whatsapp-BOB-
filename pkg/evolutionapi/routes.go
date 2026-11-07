package evolutionapi

import "github.com/gin-gonic/gin"

// Register mounts every Evolution API v2 route. POST /instance/create and
// DELETE /instance/delete/:instanceId are shared with the native API and are
// wired through CreateOrNative / DeleteOrNative in pkg/routes instead.
func (h *Handler) Register(eng *gin.Engine) {
	eng.GET("/instance/fetchInstances", h.FetchInstances)

	inst := eng.Group("", h.requireInstance)

	i := inst.Group("/instance")
	i.GET("/connect/:instance", h.Connect)
	i.GET("/connectionState/:instance", h.ConnectionState)
	i.POST("/restart/:instance", h.Restart)
	i.PUT("/restart/:instance", h.Restart)
	i.POST("/setPresence/:instance", h.SetPresence)
	i.DELETE("/logout/:instance", h.Logout)

	s := inst.Group("/settings")
	s.POST("/set/:instance", h.SettingsSet)
	s.GET("/find/:instance", h.SettingsFind)

	w := inst.Group("/webhook")
	w.POST("/set/:instance", h.WebhookSet)
	w.GET("/find/:instance", h.WebhookFind)

	p := inst.Group("/proxy")
	p.POST("/set/:instance", h.ProxySet)
	p.GET("/find/:instance", h.ProxyFind)

	m := inst.Group("/message")
	m.POST("/sendText/:instance", h.SendText)
	m.POST("/sendMedia/:instance", h.SendMedia)
	m.POST("/sendPtv/:instance", h.SendPtv)
	m.POST("/sendWhatsAppAudio/:instance", h.SendWhatsAppAudio)
	m.POST("/sendSticker/:instance", h.SendSticker)
	m.POST("/sendLocation/:instance", h.SendLocation)
	m.POST("/sendContact/:instance", h.SendContact)
	m.POST("/sendReaction/:instance", h.SendReaction)
	m.POST("/sendPoll/:instance", h.SendPoll)
	m.POST("/sendList/:instance", h.SendList)
	m.POST("/sendButtons/:instance", h.SendButtons)
	m.POST("/sendStatus/:instance", h.SendStatus)

	ch := inst.Group("/chat")
	ch.POST("/whatsappNumbers/:instance", h.WhatsappNumbers)
	ch.POST("/markMessageAsRead/:instance", h.MarkMessageAsRead)
	ch.POST("/markChatUnread/:instance", h.MarkChatUnread)
	ch.POST("/archiveChat/:instance", h.ArchiveChat)
	ch.DELETE("/deleteMessageForEveryone/:instance", h.DeleteMessageForEveryone)
	ch.POST("/updateMessage/:instance", h.UpdateMessage)
	ch.POST("/sendPresence/:instance", h.SendPresence)
	ch.POST("/updateBlockStatus/:instance", h.UpdateBlockStatus)
	ch.POST("/fetchProfilePictureUrl/:instance", h.FetchProfilePictureUrl)
	ch.POST("/fetchProfile/:instance", h.FetchProfile)
	ch.POST("/fetchBusinessProfile/:instance", h.FetchBusinessProfile)
	ch.POST("/findContacts/:instance", h.FindContacts)
	ch.POST("/getBase64FromMediaMessage/:instance", h.GetBase64FromMediaMessage)
	ch.POST("/updateProfileName/:instance", h.UpdateProfileName)
	ch.POST("/updateProfileStatus/:instance", h.UpdateProfileStatus)
	ch.POST("/updateProfilePicture/:instance", h.UpdateProfilePicture)
	ch.DELETE("/removeProfilePicture/:instance", h.RemoveProfilePicture)
	ch.GET("/fetchPrivacySettings/:instance", h.FetchPrivacySettings)
	ch.POST("/updatePrivacySettings/:instance", h.UpdatePrivacySettings)

	g := inst.Group("/group")
	g.POST("/create/:instance", h.GroupCreate)
	g.POST("/updateGroupSubject/:instance", h.GroupUpdateSubject)
	g.POST("/updateGroupDescription/:instance", h.GroupUpdateDescription)
	g.POST("/updateGroupPicture/:instance", h.GroupUpdatePicture)
	g.GET("/findGroupInfos/:instance", h.GroupFindInfos)
	g.GET("/fetchAllGroups/:instance", h.GroupFetchAll)
	g.GET("/participants/:instance", h.GroupParticipants)
	g.GET("/inviteCode/:instance", h.GroupInviteCode)
	g.POST("/revokeInviteCode/:instance", h.GroupRevokeInviteCode)
	g.PUT("/revokeInviteCode/:instance", h.GroupRevokeInviteCode)
	g.GET("/inviteInfo/:instance", h.GroupInviteInfo)
	g.GET("/acceptInviteCode/:instance", h.GroupAcceptInviteCode)
	g.POST("/sendInvite/:instance", h.GroupSendInvite)
	g.POST("/updateParticipant/:instance", h.GroupUpdateParticipant)
	g.POST("/updateSetting/:instance", h.GroupUpdateSetting)
	g.POST("/toggleEphemeral/:instance", h.GroupToggleEphemeral)
	g.DELETE("/leaveGroup/:instance", h.GroupLeave)

	l := inst.Group("/label")
	l.GET("/findLabels/:instance", h.FindLabels)
	l.POST("/handleLabel/:instance", h.HandleLabel)

	inst.POST("/call/offer/:instance", h.OfferCall)
}
