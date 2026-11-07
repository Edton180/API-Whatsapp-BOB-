// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"github.com/edton180/api-whatsapp-bob/internal/media"
)

// ---------------------------------------------------------------- ações em mensagens

// React reage a uma mensagem (emoji vazio remove a reação).
func (s *Session) React(ctx context.Context, number, id, emoji string, fromMe bool, participant string) (map[string]any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	chat, err := s.ResolveJID(ctx, number)
	if err != nil {
		return nil, err
	}
	sender := chat
	if fromMe && c.Store.ID != nil {
		sender = c.Store.ID.ToNonAD()
	} else if participant != "" {
		if sender, err = s.ResolveJID(ctx, participant); err != nil {
			return nil, err
		}
	}
	return s.Send(ctx, chat, c.BuildReaction(chat, sender, types.MessageID(id), emoji), SendOptions{})
}

// MarkRead marca mensagens como lidas.
func (s *Session) MarkRead(ctx context.Context, number string, ids []string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	chat, err := s.ResolveJID(ctx, number)
	if err != nil {
		return err
	}
	list := make([]types.MessageID, 0, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			list = append(list, types.MessageID(id))
		}
	}
	if len(list) == 0 {
		return errors.New("informe os ids das mensagens")
	}
	return c.MarkRead(ctx, list, time.Now(), chat, chat)
}

// Revoke apaga para todos uma mensagem enviada pela instância.
func (s *Session) Revoke(ctx context.Context, chat, id string) (map[string]any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	to, err := s.ResolveJID(ctx, chat)
	if err != nil {
		return nil, err
	}
	return s.Send(ctx, to, c.BuildRevoke(to, types.EmptyJID, types.MessageID(id)), SendOptions{})
}

// Edit troca o texto de uma mensagem enviada (até 15 minutos depois).
func (s *Session) Edit(ctx context.Context, chat, id, text string) (map[string]any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	to, err := s.ResolveJID(ctx, chat)
	if err != nil {
		return nil, err
	}
	return s.Send(ctx, to, c.BuildEdit(to, types.MessageID(id), Text(text)), SendOptions{})
}

// Presence mostra "digitando...", "gravando áudio..." ou pausa.
func (s *Session) Presence(ctx context.Context, number, state string, isAudio bool, delay int) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	to, err := s.ResolveJID(ctx, number)
	if err != nil {
		return err
	}
	switch strings.ToLower(state) {
	case "available", "unavailable":
		return c.SendPresence(ctx, types.Presence(strings.ToLower(state)))
	}
	cp, mediaType := types.ChatPresenceComposing, types.ChatPresenceMediaText
	if strings.ToLower(state) == "paused" {
		cp = types.ChatPresencePaused
	}
	if isAudio || strings.ToLower(state) == "recording" {
		cp, mediaType = types.ChatPresenceComposing, types.ChatPresenceMediaAudio
	}
	if err := c.SendChatPresence(ctx, to, cp, mediaType); err != nil {
		return err
	}
	if delay > 0 && cp == types.ChatPresenceComposing {
		go func() {
			time.Sleep(time.Duration(min(delay, 60000)) * time.Millisecond)
			_ = c.SendChatPresence(context.Background(), to, types.ChatPresencePaused, mediaType)
		}()
	}
	return nil
}

// DownloadMedia baixa a mídia de uma mensagem (o JSON "Message" recebido no webhook ou o id
// de uma mensagem recente).
func (s *Session) DownloadMedia(ctx context.Context, raw json.RawMessage, id string) ([]byte, string, error) {
	c, err := s.Client()
	if err != nil {
		return nil, "", err
	}
	var msg *waE2E.Message
	if len(raw) > 2 {
		msg = &waE2E.Message{}
		if err := json.Unmarshal(raw, msg); err != nil {
			return nil, "", fmt.Errorf("mensagem inválida: %w", err)
		}
	} else if id != "" {
		msg = s.lookup(id)
	}
	if msg == nil {
		return nil, "", errors.New("mensagem não encontrada: envie o campo message recebido no webhook")
	}
	data, mime := downloadMedia(c, msg)
	if data == nil {
		return nil, "", errors.New("a mensagem não tem mídia ou o arquivo expirou")
	}
	return data, mime, nil
}

// ---------------------------------------------------------------- usuários

// Check confere quais números têm WhatsApp.
func (s *Session) Check(ctx context.Context, numbers []string) ([]map[string]any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	q := make([]string, 0, len(numbers))
	for _, n := range numbers {
		if d := digits(n); d != "" {
			q = append(q, "+"+d)
		}
	}
	if len(q) == 0 {
		return nil, errors.New("informe os números")
	}
	res, err := c.IsOnWhatsApp(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(res))
	for _, r := range res {
		row := map[string]any{"Query": r.Query, "IsInWhatsapp": r.IsIn, "JID": r.JID.String()}
		if r.VerifiedName != nil && r.VerifiedName.Details != nil {
			row["VerifiedName"] = r.VerifiedName.Details.GetVerifiedName()
		}
		if r.IsIn {
			s.jids.Store(digits(r.Query), r.JID)
		}
		out = append(out, row)
	}
	return out, nil
}

// UserInfo devolve recado, foto e dados comerciais dos números.
func (s *Session) UserInfo(ctx context.Context, numbers []string) (map[string]any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jids := []types.JID{}
	for _, n := range numbers {
		j, err := s.ResolveJID(ctx, n)
		if err != nil {
			return nil, err
		}
		jids = append(jids, j)
	}
	info, err := c.GetUserInfo(ctx, jids)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for j, u := range info {
		out[j.String()] = u
	}
	return out, nil
}

// Avatar devolve a URL da foto de perfil (vazia quando não há ou é privada).
func (s *Session) Avatar(ctx context.Context, number string, preview bool) (map[string]any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := s.ResolveJID(ctx, number)
	if err != nil {
		return nil, err
	}
	pic, err := c.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: preview})
	if err != nil || pic == nil {
		return map[string]any{"url": nil}, nil
	}
	return map[string]any{"url": pic.URL, "id": pic.ID, "type": pic.Type}, nil
}

// Contacts lista a agenda sincronizada do aparelho.
func (s *Session) Contacts(ctx context.Context) ([]map[string]any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	all, err := c.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(all))
	for j, ci := range all {
		out = append(out, map[string]any{
			"Jid": j.String(), "Found": ci.Found, "FirstName": ci.FirstName, "FullName": ci.FullName,
			"PushName": ci.PushName, "BusinessName": ci.BusinessName,
		})
	}
	return out, nil
}

// Block bloqueia ou desbloqueia um contato.
func (s *Session) Block(ctx context.Context, number string, block bool) (any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := s.ResolveJID(ctx, number)
	if err != nil {
		return nil, err
	}
	action := events.BlocklistChangeActionBlock
	if !block {
		action = events.BlocklistChangeActionUnblock
	}
	return c.UpdateBlocklist(ctx, jid, action, "")
}

// Blocklist lista os contatos bloqueados.
func (s *Session) Blocklist(ctx context.Context) (any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	return c.GetBlocklist(ctx, "")
}

// Privacy lê as configurações de privacidade.
func (s *Session) Privacy(ctx context.Context) (any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	return c.GetPrivacySettings(ctx), nil
}

// ProfileName troca o nome do perfil.
func (s *Session) ProfileName(ctx context.Context, name string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		return errors.New("informe o nome")
	}
	return c.SendAppState(ctx, appstate.BuildSettingPushName(name))
}

// ProfileStatus troca o recado do perfil.
func (s *Session) ProfileStatus(ctx context.Context, status string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	return c.SetStatusMessage(ctx, types.SetStatusInput{Text: proto.String(status)})
}

// ProfilePicture troca a foto do perfil (ou de um grupo, se jid for informado).
func (s *Session) ProfilePicture(ctx context.Context, image string, group string) (string, error) {
	c, err := s.Client()
	if err != nil {
		return "", err
	}
	data, _, err := media.Load(ctx, image)
	if err != nil {
		return "", err
	}
	jpg, err := media.ImageJPEG(data)
	if err != nil {
		return "", err
	}
	target := types.EmptyJID
	if group != "" {
		if target, err = s.ResolveJID(ctx, group); err != nil {
			return "", err
		}
	}
	return c.SetGroupPhoto(ctx, target, jpg)
}

// ---------------------------------------------------------------- conversas

// Chat arquiva, fixa ou silencia uma conversa.
func (s *Session) Chat(ctx context.Context, action, chat string, muteHours int) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := s.ResolveJID(ctx, chat)
	if err != nil {
		return err
	}
	var patch appstate.PatchInfo
	switch action {
	case "archive", "unarchive":
		patch = appstate.BuildArchive(jid, action == "archive", time.Now(), nil)
	case "pin", "unpin":
		patch = appstate.BuildPin(jid, action == "pin")
	case "mute", "unmute":
		d := time.Duration(muteHours) * time.Hour
		if d <= 0 {
			d = 8 * time.Hour
		}
		patch = appstate.BuildMute(jid, action == "mute", d)
	case "read", "unread":
		patch = appstate.BuildMarkChatAsRead(jid, action == "read", time.Now(), nil)
	default:
		return fmt.Errorf("ação desconhecida: %s", action)
	}
	if err := c.SendAppState(ctx, patch); err != nil {
		return err
	}
	s.chatChanged(jid, action)
	return nil
}

// ---------------------------------------------------------------- etiquetas

// Label coloca ou tira uma etiqueta de uma conversa.
func (s *Session) Label(ctx context.Context, chat, labelID string, add bool) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := s.ResolveJID(ctx, chat)
	if err != nil {
		return err
	}
	if labelID == "" {
		return errors.New("informe o labelId")
	}
	return c.SendAppState(ctx, appstate.BuildLabelChat(jid, labelID, add))
}

// LabelMessage coloca ou tira uma etiqueta de uma mensagem.
func (s *Session) LabelMessage(ctx context.Context, chat, labelID, messageID string, add bool) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := s.ResolveJID(ctx, chat)
	if err != nil {
		return err
	}
	return c.SendAppState(ctx, appstate.BuildLabelMessage(jid, labelID, messageID, add))
}

// LabelEdit cria, renomeia ou apaga uma etiqueta.
func (s *Session) LabelEdit(ctx context.Context, labelID, name string, color int32, deleted bool) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	if labelID == "" {
		labelID = fmt.Sprintf("%d", time.Now().Unix()%100000)
	}
	return c.SendAppState(ctx, appstate.BuildLabelEdit(labelID, name, color, deleted))
}

// ---------------------------------------------------------------- grupos

func (s *Session) groupJID(ctx context.Context, v string) (types.JID, error) {
	if v == "" {
		return types.EmptyJID, errors.New("informe o groupJid")
	}
	if !strings.Contains(v, "@") {
		v = strings.TrimSuffix(v, "-group") + "@" + types.GroupServer
	}
	return types.ParseJID(v)
}

func (s *Session) participants(ctx context.Context, list []string) ([]types.JID, error) {
	out := make([]types.JID, 0, len(list))
	for _, p := range list {
		j, err := s.ResolveJID(ctx, p)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, nil
}

// Groups lista os grupos em que o número participa.
func (s *Session) Groups(ctx context.Context) (any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	return c.GetJoinedGroups(ctx)
}

// GroupInfo devolve os dados de um grupo.
func (s *Session) GroupInfo(ctx context.Context, group string) (any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return nil, err
	}
	return c.GetGroupInfo(ctx, jid)
}

// GroupCreate cria um grupo.
func (s *Session) GroupCreate(ctx context.Context, name string, members []string) (any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("informe o nome do grupo")
	}
	ps, err := s.participants(ctx, members)
	if err != nil {
		return nil, err
	}
	return c.CreateGroup(ctx, whatsmeow.ReqCreateGroup{Name: name, Participants: ps})
}

// GroupParticipants adiciona, remove, promove ou rebaixa participantes.
func (s *Session) GroupParticipants(ctx context.Context, group, action string, members []string) (any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return nil, err
	}
	ps, err := s.participants(ctx, members)
	if err != nil {
		return nil, err
	}
	switch action {
	case "add", "remove", "promote", "demote":
	default:
		return nil, errors.New("action deve ser add, remove, promote ou demote")
	}
	return c.UpdateGroupParticipants(ctx, jid, ps, whatsmeow.ParticipantChange(action))
}

// GroupName troca o nome do grupo.
func (s *Session) GroupName(ctx context.Context, group, name string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return err
	}
	return c.SetGroupName(ctx, jid, name)
}

// GroupDescription troca a descrição do grupo.
func (s *Session) GroupDescription(ctx context.Context, group, description string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return err
	}
	return c.SetGroupDescription(ctx, jid, description)
}

// GroupInvite devolve o link de convite (reset gera um novo).
func (s *Session) GroupInvite(ctx context.Context, group string, reset bool) (string, error) {
	c, err := s.Client()
	if err != nil {
		return "", err
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return "", err
	}
	return c.GetGroupInviteLink(ctx, jid, reset)
}

// GroupJoin entra num grupo pelo código ou link de convite.
func (s *Session) GroupJoin(ctx context.Context, code string) (string, error) {
	c, err := s.Client()
	if err != nil {
		return "", err
	}
	code = strings.TrimPrefix(strings.TrimSpace(code), "https://chat.whatsapp.com/")
	jid, err := c.JoinGroupWithLink(ctx, code)
	return jid.String(), err
}

// GroupLeave sai do grupo.
func (s *Session) GroupLeave(ctx context.Context, group string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return err
	}
	return c.LeaveGroup(ctx, jid)
}

// Newsletters lista os canais seguidos.
func (s *Session) Newsletters(ctx context.Context) (any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	return c.GetSubscribedNewsletters(ctx)
}

var _ = proto.String
