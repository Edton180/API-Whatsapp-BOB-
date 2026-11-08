// API WhatsApp BOB - Desenvolvido por Edton Braz.

package events

import "strings"

// Nome de cada evento da BOB no padrão v2 (usado no webhook com um endereço por evento).
var v2Names = map[string]string{
	"Message": "MESSAGES_UPSERT", "UndecryptableMessage": "MESSAGES_UPSERT", "MediaRetry": "MESSAGES_UPDATE",
	"PollVote": "MESSAGES_UPDATE", "Receipt": "MESSAGES_UPDATE", "SendMessage": "SEND_MESSAGE",
	"DeleteForMe": "MESSAGES_DELETE",
	"Presence":    "PRESENCE_UPDATE", "ChatPresence": "PRESENCE_UPDATE",
	"HistorySync": "MESSAGES_SET",
	"CallOffer":   "CALL", "CallAccept": "CALL", "CallTerminate": "CALL", "CallOfferNotice": "CALL", "CallRelayLatency": "CALL",
	"QRCode": "QRCODE_UPDATED", "QRTimeout": "QRCODE_UPDATED", "QRSuccess": "QRCODE_UPDATED",
	"Contact": "CONTACTS_UPDATE", "PushName": "CONTACTS_UPDATE", "Picture": "CONTACTS_UPDATE",
	"BusinessName": "CONTACTS_UPDATE", "UserAbout": "CONTACTS_UPDATE",
	"GroupInfo": "GROUP_UPDATE", "JoinedGroup": "GROUPS_UPSERT",
	"LabelEdit": "LABELS_EDIT", "LabelAssociationChat": "LABELS_ASSOCIATION", "LabelAssociationMessage": "LABELS_ASSOCIATION",
	"Archive": "CHATS_UPDATE", "Pin": "CHATS_UPDATE", "Mute": "CHATS_UPDATE", "Star": "CHATS_UPDATE",
	"MarkChatAsRead": "CHATS_UPDATE", "ClearChat": "CHATS_UPDATE", "DeleteChat": "CHATS_DELETE",
	"NewsletterJoin": "NEWSLETTER", "NewsletterLeave": "NEWSLETTER", "NewsletterMuteChange": "NEWSLETTER", "NewsletterLiveUpdate": "NEWSLETTER",
	"Blocklist": "BLOCKLIST",
}

// V2Name devolve o nome do evento no padrão v2 (MESSAGES_UPSERT, CONNECTION_UPDATE...).
func V2Name(event string) string {
	if n, ok := v2Names[event]; ok {
		return n
	}
	if categoryOf(event) == "CONNECTION" {
		return "CONNECTION_UPDATE"
	}
	return strings.ToUpper(event)
}

// V2Path é o final do endereço no webhook por evento: MESSAGES_UPSERT -> messages-upsert.
func V2Path(event string) string {
	return strings.ToLower(strings.ReplaceAll(V2Name(event), "_", "-"))
}

func categoryOf(event string) string {
	for cat, list := range categories {
		for _, e := range list {
			if e == event {
				return cat
			}
		}
	}
	return ""
}

// Target diz para onde vai o evento de uma instância.
type Target struct {
	URL        string // webhook (vazio = nenhum)
	Subscribed string // categorias inscritas (vazio = todas)
	ByEvents   bool   // um endereço por evento: URL + "/" + V2Path(evento)
}

// URLFor devolve o endereço que recebe o evento.
func (t Target) URLFor(event string) string {
	if t.URL == "" || !t.ByEvents {
		return t.URL
	}
	return strings.TrimRight(t.URL, "/") + "/" + V2Path(event)
}
