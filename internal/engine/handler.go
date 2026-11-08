// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	bobstore "github.com/edton180/api-whatsapp-bob/internal/store"
)

// handle recebe os eventos da whatsmeow e publica no formato da BOB.
func (s *Session) handle(raw any) {
	s.touch()
	switch v := raw.(type) {
	case *events.Message:
		s.onMessage(v)
	case *events.Receipt:
		s.onReceipt(v)
		s.emit("Receipt", v)
	case *events.Connected:
		s.onConnected()
	case *events.PairSuccess:
		_ = s.e.Repo.Update(s.ID, map[string]any{"jid": v.ID.String(), "qrcode": ""})
		s.mu.Lock()
		s.inst.Jid = v.ID.String()
		s.mu.Unlock()
		s.clearQR()
		s.stopQR()
		s.logf("INFO", "aparelho pareado: %s", v.ID.User)
		s.emit("PairSuccess", map[string]any{"jid": v.ID.String(), "lid": v.LID.String(), "businessName": v.BusinessName, "platform": v.Platform})
	case *events.PairError:
		s.emit("PairError", map[string]any{"jid": v.ID.String(), "error": errString(v.Error)})
	case *events.LoggedOut:
		reason := v.Reason.String()
		s.logf("WARN", "sessão encerrada pelo WhatsApp: %s", reason)
		_ = s.e.Repo.Update(s.ID, map[string]any{"jid": "", "connected": false, "disconnect_reason": reason})
		s.mu.Lock()
		s.inst.Jid = ""
		s.cli = nil
		s.mu.Unlock()
		s.emit("LoggedOut", map[string]any{"reason": reason, "onConnect": v.OnConnect})
	case *events.Disconnected:
		_ = s.e.Repo.Update(s.ID, map[string]any{"connected": false})
		s.emit("Disconnected", map[string]any{})
	case *events.StreamReplaced:
		s.logf("WARN", "conexão substituída por outra sessão com o mesmo aparelho")
		s.emit("StreamReplaced", map[string]any{})
	case *events.ConnectFailure:
		s.logf("ERROR", "conexão recusada: %s %s", v.Reason.String(), v.Message)
		_ = s.e.Repo.Update(s.ID, map[string]any{"connected": false, "disconnect_reason": v.Reason.String()})
		s.emit("ConnectFailure", map[string]any{"reason": v.Reason.String(), "message": v.Message})
	case *events.TemporaryBan:
		s.logf("ERROR", "banimento temporário: %s até %s", v.Code.String(), v.Expire.String())
		s.emit("TemporaryBan", map[string]any{"reason": v.Code.String(), "expire": v.Expire.String()})
	case *events.ClientOutdated:
		s.onOutdated()
		s.emit("ClientOutdated", map[string]any{})
	case *events.CallOffer:
		s.onCall(v)
	case *events.LabelEdit:
		if v.Action != nil {
			_ = s.e.Repo.SaveLabel(&bobstore.Label{
				InstanceID: s.ID, LabelID: v.LabelID, Name: v.Action.GetName(),
				Color: v.Action.GetColor(), Deleted: v.Action.GetDeleted(),
			})
		}
		s.emit("LabelEdit", v)
	case *events.HistorySync:
		// O histórico completo é grande demais para um webhook: só o resumo.
		// gravar o histórico pode demorar: fica fora da fila de eventos da whatsmeow
		go s.onHistory(v)
		d := v.Data
		s.emit("HistorySync", map[string]any{"syncType": d.GetSyncType().String(), "conversations": len(d.GetConversations()), "progress": d.GetProgress()})
	case *events.QR, *events.AppStateSyncComplete, *events.AppState, *events.KeepAliveTimeout, *events.KeepAliveRestored:
		// tratados internamente
	default:
		s.onChatSync(raw)
		name := eventName(raw)
		if name != "" && knownEvents[name] {
			s.emit(name, raw)
		}
	}
}

var knownEvents = map[string]bool{
	"Presence": true, "ChatPresence": true, "PushName": true, "Picture": true, "BusinessName": true,
	"UserAbout": true, "Contact": true, "GroupInfo": true, "JoinedGroup": true, "CallAccept": true,
	"CallTerminate": true, "CallOfferNotice": true, "LabelAssociationChat": true, "LabelAssociationMessage": true,
	"Archive": true, "Pin": true, "Mute": true, "Star": true, "DeleteChat": true, "DeleteForMe": true,
	"MarkChatAsRead": true, "ClearChat": true, "Blocklist": true, "NewsletterJoin": true, "NewsletterLeave": true,
	"NewsletterMuteChange": true, "NewsletterLiveUpdate": true, "UndecryptableMessage": true, "MediaRetry": true,
	"OfflineSyncCompleted": true, "IdentityChange": true, "PrivacySettings": true,
}

func eventName(v any) string {
	t := strings.TrimPrefix(strings.TrimPrefix(typeName(v), "*"), "events.")
	if strings.Contains(t, ".") {
		return ""
	}
	return t
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *Session) onConnected() {
	c := s.client()
	if c == nil || c.Store == nil || c.Store.ID == nil {
		return
	}
	inst := s.Instance()
	jid := c.Store.ID.String()
	_ = s.e.Repo.Update(s.ID, map[string]any{"jid": jid, "connected": true, "disconnect_reason": "", "qrcode": ""})
	s.mu.Lock()
	s.inst.Jid, s.inst.Connected = jid, true
	s.mu.Unlock()
	s.clearQR()
	if inst.AlwaysOnline {
		_ = c.SendPresence(context.Background(), types.PresenceAvailable)
	} else {
		_ = c.SendPresence(context.Background(), types.PresenceUnavailable)
	}
	s.logf("INFO", "conectada como %s (%s)", c.Store.ID.User, c.Store.PushName)
	s.emit("Connected", map[string]any{"jid": jid, "pushName": c.Store.PushName, "status": "open"})
}

func (s *Session) onCall(v *events.CallOffer) {
	inst := s.Instance()
	if inst.RejectCall {
		if c := s.client(); c != nil {
			_ = c.RejectCall(context.Background(), v.From, v.CallID)
			if msg := strings.TrimSpace(inst.MsgRejectCall); msg != "" {
				to := v.From.ToNonAD()
				go func() {
					_, _ = s.Send(context.Background(), to, &waE2E.Message{Conversation: &msg}, SendOptions{})
				}()
			}
			s.logf("INFO", "chamada de %s recusada", v.From.User)
		}
	}
	s.emit("CallOffer", v)
}

func (s *Session) onMessage(v *events.Message) {
	inst := s.Instance()
	chat := v.Info.Chat
	if chat.Server == types.BroadcastServer && (inst.IgnoreStatus || s.e.Cfg.IgnoreStatus) && chat.User == "status" {
		return
	}
	if v.Info.IsGroup && (inst.IgnoreGroups || s.e.Cfg.IgnoreGroups) {
		return
	}
	s.recent.put(v.Info.ID, v.Message)
	status := "DELIVERY_ACK"
	if v.Info.IsFromMe {
		status = "SERVER_ACK"
	}
	s.record(v.Info, v.Message, status, false)
	if v.Message.GetPollUpdateMessage() != nil {
		s.onPollVote(v)
	}
	if inst.ReadMessages && !v.Info.IsFromMe {
		if c := s.client(); c != nil {
			_ = c.MarkRead(context.Background(), []types.MessageID{v.Info.ID}, time.Now(), chat, v.Info.Sender)
		}
	}
	data := map[string]any{}
	if b, err := json.Marshal(v); err == nil {
		_ = json.Unmarshal(b, &data)
	}
	delete(data, "RawMessage")
	if inst.Base64(s.e.Cfg.WebhookFiles) {
		if msg, ok := data["Message"].(map[string]any); ok {
			if c := s.client(); c != nil {
				if file, mime := downloadMedia(c, v.Message); file != nil {
					msg["base64"] = base64.StdEncoding.EncodeToString(file)
					msg["mimetype"] = mime
				}
			}
		}
	}
	s.emit("Message", data)
}

// downloadMedia baixa e decifra a mídia de uma mensagem recebida (até 100 MB).
func downloadMedia(c *whatsmeow.Client, m *waE2E.Message) ([]byte, string) {
	m = unwrap(m)
	if m == nil {
		return nil, ""
	}
	var d whatsmeow.DownloadableMessage
	var mime string
	var size uint64
	switch {
	case m.ImageMessage != nil:
		d, mime, size = m.ImageMessage, m.ImageMessage.GetMimetype(), m.ImageMessage.GetFileLength()
	case m.VideoMessage != nil:
		d, mime, size = m.VideoMessage, m.VideoMessage.GetMimetype(), m.VideoMessage.GetFileLength()
	case m.AudioMessage != nil:
		d, mime, size = m.AudioMessage, m.AudioMessage.GetMimetype(), m.AudioMessage.GetFileLength()
	case m.DocumentMessage != nil:
		d, mime, size = m.DocumentMessage, m.DocumentMessage.GetMimetype(), m.DocumentMessage.GetFileLength()
	case m.DocumentWithCaptionMessage.GetMessage().GetDocumentMessage() != nil:
		dm := m.DocumentWithCaptionMessage.GetMessage().GetDocumentMessage()
		d, mime, size = dm, dm.GetMimetype(), dm.GetFileLength()
	case m.StickerMessage != nil:
		d, mime, size = m.StickerMessage, m.StickerMessage.GetMimetype(), m.StickerMessage.GetFileLength()
	default:
		return nil, ""
	}
	if size > 100<<20 {
		return nil, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	data, err := c.Download(ctx, d)
	if err != nil {
		return nil, ""
	}
	return data, mime
}

// unwrap tira as camadas de mensagem temporária e visualização única.
func unwrap(m *waE2E.Message) *waE2E.Message {
	for i := 0; i < 4 && m != nil; i++ {
		switch {
		case m.EphemeralMessage.GetMessage() != nil:
			m = m.EphemeralMessage.GetMessage()
		case m.ViewOnceMessage.GetMessage() != nil:
			m = m.ViewOnceMessage.GetMessage()
		case m.ViewOnceMessageV2.GetMessage() != nil:
			m = m.ViewOnceMessageV2.GetMessage()
		case m.ViewOnceMessageV2Extension.GetMessage() != nil:
			m = m.ViewOnceMessageV2Extension.GetMessage()
		default:
			return m
		}
	}
	return m
}

// ---------------------------------------------------------------- cache de mensagens

// messageCache guarda as últimas mensagens para responder citando (quoted) e baixar mídia.
type messageCache struct {
	mu    sync.Mutex
	max   int
	order []string
	data  map[string]*waE2E.Message
}

func newMessageCache(max int) *messageCache {
	return &messageCache{max: max, data: map[string]*waE2E.Message{}}
}

func (c *messageCache) put(id string, m *waE2E.Message) {
	if id == "" || m == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.data[id]; !ok {
		c.order = append(c.order, id)
	}
	c.data[id] = m
	for len(c.order) > c.max {
		delete(c.data, c.order[0])
		c.order = c.order[1:]
	}
}

func (c *messageCache) get(id string) *waE2E.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data[id]
}
