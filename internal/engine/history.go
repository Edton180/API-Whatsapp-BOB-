// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

// Histórico: guarda no banco as mensagens recebidas, enviadas e sincronizadas do celular,
// o status de entrega de cada uma, as conversas e os votos das enquetes.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	bobstore "github.com/edton180/api-whatsapp-bob/internal/store"
)

// storing diz se o histórico está ligado.
func (s *Session) storing() bool { return s.e.Cfg.StoreMessages }

// record guarda uma mensagem e atualiza a conversa. Edição e "apagar para todos" mudam a
// mensagem original em vez de criar outra.
func (s *Session) record(info types.MessageInfo, msg *waE2E.Message, status string, history bool) {
	if !s.storing() || msg == nil || info.ID == "" {
		return
	}
	inner := unwrap(msg)
	if pm := inner.GetProtocolMessage(); pm != nil {
		target := pm.GetKey().GetID()
		switch pm.GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			if target != "" {
				_ = s.e.Repo.MarkDeleted(s.ID, target)
			}
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			if edited := pm.GetEditedMessage(); edited != nil && target != "" {
				raw, _ := proto.Marshal(edited)
				_ = s.e.Repo.EditMessage(s.ID, target, MessageText(edited), raw)
			}
		}
		return
	}
	kind := MessageKind(inner)
	switch kind {
	case "", "reactionMessage", "pollUpdateMessage", "senderKeyDistributionMessage", "keepInChatMessage",
		"encReactionMessage", "messageContextInfo":
		return
	}
	raw, err := proto.Marshal(msg)
	if err != nil {
		return
	}
	ts := info.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	m := &bobstore.Message{
		InstanceID: s.ID, ID: info.ID, Chat: info.Chat.ToNonAD().String(), Sender: info.Sender.ToNonAD().String(),
		FromMe: info.IsFromMe, PushName: info.PushName, Type: kind, Text: MessageText(inner),
		Status: status, Timestamp: ts.Unix(), Raw: raw,
	}
	if err := s.e.Repo.SaveMessage(m); err != nil {
		s.logf("WARN", "histórico: não salvei a mensagem %s: %v", info.ID, err)
		return
	}
	if info.Chat.Server == types.BroadcastServer {
		return
	}
	name := ""
	if !info.IsFromMe && !info.IsGroup {
		name = info.PushName
	}
	_ = s.e.Repo.TouchChat(s.ID, m.Chat, name, m, !info.IsFromMe && !history)
}

// onReceipt avança o status das mensagens enviadas e zera as não lidas quando o próprio
// usuário lê a conversa em outro aparelho.
func (s *Session) onReceipt(v *events.Receipt) {
	if !s.storing() {
		return
	}
	ids := make([]string, len(v.MessageIDs))
	for i, id := range v.MessageIDs {
		ids[i] = string(id)
	}
	var status string
	switch v.Type {
	case types.ReceiptTypeDelivered:
		status = "DELIVERY_ACK"
	case types.ReceiptTypeRead:
		status = "READ"
	case types.ReceiptTypePlayed:
		status = "PLAYED"
	case types.ReceiptTypeReadSelf, types.ReceiptTypePlayedSelf:
		_ = s.e.Repo.UpdateChat(s.ID, v.Chat.ToNonAD().String(), map[string]any{"unread": 0})
		return
	default:
		return
	}
	_ = s.e.Repo.UpdateStatus(s.ID, ids, status)
}

// onHistory guarda as conversas e mensagens que o celular manda ao conectar (ou a pedido).
func (s *Session) onHistory(v *events.HistorySync) {
	if !s.storing() {
		return
	}
	c := s.client()
	if c == nil {
		return
	}
	saved := 0
	for _, conv := range v.Data.GetConversations() {
		chat, err := types.ParseJID(conv.GetID())
		if err != nil || chat.Server == types.BroadcastServer {
			continue
		}
		name := conv.GetName()
		if name == "" {
			name = conv.GetDisplayName()
		}
		_ = s.e.Repo.SaveChat(&bobstore.Chat{
			InstanceID: s.ID, JID: chat.ToNonAD().String(), Name: name,
			Unread: int(conv.GetUnreadCount()), Archived: conv.GetArchived(), Pinned: conv.GetPinned() > 0,
			LastTimestamp: int64(conv.GetConversationTimestamp()),
		})
		for _, hm := range conv.GetMessages() {
			web := hm.GetMessage()
			if web == nil {
				continue
			}
			evt, err := c.ParseWebMessage(chat, web)
			if err != nil || evt.Message == nil {
				continue
			}
			// o status do celular já usa os nomes PENDING, SERVER_ACK, DELIVERY_ACK, READ, PLAYED e ERROR
			status := web.GetStatus().String()
			if bobstore.StatusRank(status) < 0 {
				status = ""
			}
			s.record(evt.Info, evt.Message, status, true)
			saved++
		}
	}
	if saved > 0 {
		s.logf("INFO", "histórico: %d mensagens sincronizadas do celular", saved)
	}
}

// lookup acha uma mensagem pelo id: primeiro no cache, depois no histórico.
func (s *Session) lookup(id string) *waE2E.Message {
	if m := s.recent.get(id); m != nil {
		return m
	}
	if !s.storing() || id == "" {
		return nil
	}
	row, err := s.e.Repo.MessageByID(s.ID, id)
	if err != nil {
		return nil
	}
	return DecodeMessage(row.Raw)
}

// lastMessage devolve a última mensagem guardada da conversa (para arquivar e marcar como lida).
func (s *Session) lastMessage(chat types.JID) *LastMessage {
	if !s.storing() {
		return nil
	}
	c, err := s.e.Repo.ChatByJID(s.ID, chat.ToNonAD().String())
	if err != nil || c.LastMessageID == "" {
		return nil
	}
	return &LastMessage{ID: c.LastMessageID, FromMe: c.LastFromMe, Sender: c.LastSender, Timestamp: time.Unix(c.LastTimestamp, 0)}
}

// ---------------------------------------------------------------- enquetes

// onPollVote decifra o voto, traduz para o nome das opções e avisa com o evento PollVote.
func (s *Session) onPollVote(v *events.Message) {
	c := s.client()
	if c == nil {
		return
	}
	vote, err := c.DecryptPollVote(context.Background(), v)
	if err != nil {
		s.logf("WARN", "voto de enquete não decifrado: %v", err)
		return
	}
	key := v.Message.GetPollUpdateMessage().GetPollCreationMessageKey()
	pollID := key.GetID()
	selected := []string{}
	if poll := s.lookup(pollID); poll != nil {
		names := PollOptions(poll)
		hashes := whatsmeow.HashPollOptions(names)
		for _, h := range vote.GetSelectedOptions() {
			for i, oh := range hashes {
				if bytes.Equal(h, oh) {
					selected = append(selected, names[i])
				}
			}
		}
	} else {
		s.logf("WARN", "voto na enquete %s, que não está no histórico: não sei o nome das opções", pollID)
	}
	voter := v.Info.Sender.ToNonAD().String()
	if s.storing() {
		b, _ := json.Marshal(selected)
		_ = s.e.Repo.SavePollVote(&bobstore.PollVote{
			InstanceID: s.ID, PollID: pollID, Voter: voter, PushName: v.Info.PushName,
			Options: string(b), Timestamp: v.Info.Timestamp.Unix(),
		})
	}
	s.emit("PollVote", map[string]any{
		"pollId": pollID, "chat": v.Info.Chat.ToNonAD().String(), "voter": voter, "pushName": v.Info.PushName,
		"selectedOptions": selected, "timestamp": v.Info.Timestamp.Unix(), "messageId": v.Info.ID,
	})
}

// PollOptions devolve os nomes das opções de uma enquete.
func PollOptions(m *waE2E.Message) []string {
	m = unwrap(m)
	var p *waE2E.PollCreationMessage
	switch {
	case m.GetPollCreationMessage() != nil:
		p = m.GetPollCreationMessage()
	case m.GetPollCreationMessageV2() != nil:
		p = m.GetPollCreationMessageV2()
	case m.GetPollCreationMessageV3() != nil:
		p = m.GetPollCreationMessageV3()
	case m.GetPollCreationMessageV5() != nil:
		p = m.GetPollCreationMessageV5()
	}
	out := []string{}
	for _, o := range p.GetOptions() {
		out = append(out, o.GetOptionName())
	}
	return out
}

// PollResults soma os votos de uma enquete do histórico.
func (s *Session) PollResults(pollID string) (map[string]any, error) {
	if !s.storing() {
		return nil, ErrHistoryOff
	}
	poll := s.lookup(pollID)
	if poll == nil {
		return nil, bobstore.ErrMessageNotFound
	}
	names := PollOptions(poll)
	counts := map[string][]string{}
	for _, n := range names {
		counts[n] = []string{}
	}
	votes, err := s.e.Repo.PollVotes(s.ID, pollID)
	if err != nil {
		return nil, err
	}
	voters := []map[string]any{}
	for _, v := range votes {
		sel := v.Selected()
		for _, o := range sel {
			counts[o] = append(counts[o], v.Voter)
		}
		voters = append(voters, map[string]any{"voter": v.Voter, "pushName": v.PushName, "selectedOptions": sel, "timestamp": v.Timestamp})
	}
	options := []map[string]any{}
	for _, n := range names {
		options = append(options, map[string]any{"name": n, "votes": len(counts[n]), "voters": counts[n]})
	}
	return map[string]any{"pollId": pollID, "question": pollQuestion(poll), "options": options, "voters": voters}, nil
}

func pollQuestion(m *waE2E.Message) string {
	m = unwrap(m)
	for _, p := range []*waE2E.PollCreationMessage{m.GetPollCreationMessage(), m.GetPollCreationMessageV2(), m.GetPollCreationMessageV3(), m.GetPollCreationMessageV5()} {
		if p != nil {
			return p.GetName()
		}
	}
	return ""
}

// ErrHistoryOff é devolvido quando STORE_MESSAGES=false.
var ErrHistoryOff = errors.New("o histórico de mensagens está desligado (STORE_MESSAGES=false)")

// ---------------------------------------------------------------- pedir histórico ao celular

// RequestHistory pede ao celular mensagens mais antigas da conversa. Elas chegam depois pelo
// evento HistorySync e entram no histórico.
func (s *Session) RequestHistory(ctx context.Context, chat string, count int) (map[string]any, error) {
	if !s.storing() {
		return nil, ErrHistoryOff
	}
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := s.ResolveJID(ctx, chat)
	if err != nil {
		return nil, err
	}
	oldest, err := s.e.Repo.OldestMessage(s.ID, jid.ToNonAD().String())
	if err != nil {
		return nil, errors.New("ainda não há mensagens desta conversa no histórico para servir de ponto de partida")
	}
	if count <= 0 || count > 500 {
		count = 50
	}
	sender, _ := types.ParseJID(oldest.Sender)
	info := &types.MessageInfo{
		MessageSource: types.MessageSource{Chat: jid, IsFromMe: oldest.FromMe, IsGroup: jid.Server == types.GroupServer, Sender: sender},
		ID:            oldest.ID,
		Timestamp:     time.Unix(oldest.Timestamp, 0),
	}
	resp, err := c.SendPeerMessage(ctx, c.BuildHistorySyncRequest(info, count))
	if err != nil {
		return nil, err
	}
	return map[string]any{"requested": true, "chat": jid.String(), "count": count, "before": oldest.ID, "requestId": resp.ID}, nil
}

// purgeLoop apaga do histórico as mensagens mais antigas que STORE_MESSAGES_DAYS.
func (e *Engine) purgeLoop() {
	days := e.Cfg.StoreDays
	if !e.Cfg.StoreMessages || days <= 0 {
		return
	}
	run := func() {
		n, err := e.Repo.PurgeMessages(time.Now().AddDate(0, 0, -days))
		if err != nil {
			log.Printf("[histórico] limpeza falhou: %v", err)
		} else if n > 0 {
			log.Printf("[histórico] %d mensagens com mais de %d dias apagadas", n, days)
		}
	}
	run()
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for range t.C {
		run()
	}
}

// ---------------------------------------------------------------- tipo e texto

// MessageKind devolve o nome do campo do conteúdo (conversation, imageMessage, ...), no
// mesmo formato do JSON da mensagem.
func MessageKind(m *waE2E.Message) string {
	m = unwrap(m)
	if m == nil {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(b, &fields) != nil {
		return ""
	}
	best := ""
	for k := range fields {
		if k == "messageContextInfo" {
			continue
		}
		if best == "" || k < best {
			best = k
		}
	}
	if best == "" && len(fields) > 0 {
		return "messageContextInfo"
	}
	return lowerFirst(best)
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// MessageText extrai o texto visível (texto, legenda, título da enquete, nome do contato...).
func MessageText(m *waE2E.Message) string {
	m = unwrap(m)
	switch {
	case m == nil:
		return ""
	case m.GetConversation() != "":
		return m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetText()
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetCaption()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetCaption()
	case m.GetDocumentMessage() != nil:
		if c := m.GetDocumentMessage().GetCaption(); c != "" {
			return c
		}
		return m.GetDocumentMessage().GetFileName()
	case m.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage() != nil:
		return m.GetDocumentWithCaptionMessage().GetMessage().GetDocumentMessage().GetCaption()
	case m.GetContactMessage() != nil:
		return m.GetContactMessage().GetDisplayName()
	case m.GetLocationMessage() != nil:
		return m.GetLocationMessage().GetName()
	case m.GetButtonsResponseMessage() != nil:
		return m.GetButtonsResponseMessage().GetSelectedDisplayText()
	case m.GetListResponseMessage() != nil:
		return m.GetListResponseMessage().GetTitle()
	case m.GetTemplateButtonReplyMessage() != nil:
		return m.GetTemplateButtonReplyMessage().GetSelectedDisplayText()
	}
	if q := pollQuestion(m); q != "" {
		return q
	}
	return ""
}

// chatChanged reflete no histórico o que foi feito na conversa.
func (s *Session) chatChanged(jid types.JID, action string) {
	if !s.storing() {
		return
	}
	fields := map[string]any{}
	switch action {
	case "archive", "unarchive":
		fields["archived"] = action == "archive"
	case "pin", "unpin":
		fields["pinned"] = action == "pin"
	case "read":
		fields["unread"] = 0
	default:
		return
	}
	_ = s.e.Repo.UpdateChat(s.ID, jid.ToNonAD().String(), fields)
}

// onChatSync aplica no histórico o que o usuário fez nas conversas pelo celular.
func (s *Session) onChatSync(raw any) {
	if !s.storing() {
		return
	}
	switch v := raw.(type) {
	case *events.Archive:
		_ = s.e.Repo.UpdateChat(s.ID, v.JID.ToNonAD().String(), map[string]any{"archived": v.Action.GetArchived()})
	case *events.Pin:
		_ = s.e.Repo.UpdateChat(s.ID, v.JID.ToNonAD().String(), map[string]any{"pinned": v.Action.GetPinned()})
	case *events.MarkChatAsRead:
		if v.Action.GetRead() {
			_ = s.e.Repo.UpdateChat(s.ID, v.JID.ToNonAD().String(), map[string]any{"unread": 0})
		}
	case *events.PushName:
		if !v.JID.IsEmpty() && v.NewPushName != "" {
			_ = s.e.Repo.UpdateChat(s.ID, v.JID.ToNonAD().String(), map[string]any{"name": v.NewPushName})
		}
	}
}

// ChatJID normaliza o número ou JID de uma conversa para buscar no histórico. Com a instância
// conectada confere o número no WhatsApp (resolve o nono dígito); sem conexão usa o que veio.
func (s *Session) ChatJID(ctx context.Context, v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.Contains(v, "@") {
		if j, err := types.ParseJID(v); err == nil {
			return j.ToNonAD().String()
		}
		return v
	}
	if j, err := s.ResolveJID(ctx, v); err == nil {
		return j.ToNonAD().String()
	}
	return digits(v) + "@" + types.DefaultUserServer
}

// DecodeMessage abre o conteúdo guardado no histórico.
func DecodeMessage(raw []byte) *waE2E.Message {
	if len(raw) == 0 {
		return nil
	}
	var m waE2E.Message
	if proto.Unmarshal(raw, &m) != nil {
		return nil
	}
	return &m
}
