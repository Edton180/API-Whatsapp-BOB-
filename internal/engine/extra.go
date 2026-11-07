// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/edton180/api-whatsapp-bob/internal/media"
)

// ---------------------------------------------------------------- grupos

// GroupSetting muda quem pode enviar mensagens e editar os dados do grupo.
//
//	announcement     só administradores enviam mensagens
//	not_announcement todos enviam mensagens
//	locked           só administradores editam nome, foto e descrição
//	unlocked         todos editam nome, foto e descrição
//	approval_on      novos membros precisam de aprovação
//	approval_off     novos membros entram direto pelo link
//	admin_add        só administradores adicionam membros
//	all_member_add   todos adicionam membros
func (s *Session) GroupSetting(ctx context.Context, group, action string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return err
	}
	switch action {
	case "announcement", "not_announcement":
		return c.SetGroupAnnounce(ctx, jid, action == "announcement")
	case "locked", "unlocked":
		return c.SetGroupLocked(ctx, jid, action == "locked")
	case "approval_on", "approval_off":
		return c.SetGroupJoinApprovalMode(ctx, jid, action == "approval_on")
	case "admin_add":
		return c.SetGroupMemberAddMode(ctx, jid, types.GroupMemberAddModeAdmin)
	case "all_member_add":
		return c.SetGroupMemberAddMode(ctx, jid, types.GroupMemberAddModeAllMember)
	}
	return fmt.Errorf("ação desconhecida: %s (use announcement, not_announcement, locked, unlocked, approval_on, approval_off, admin_add ou all_member_add)", action)
}

// Ephemeral liga as mensagens temporárias de uma conversa ou grupo. Os valores aceitos pelo
// WhatsApp são 0 (desligado), 86400 (24 horas), 604800 (7 dias) e 7776000 (90 dias).
func (s *Session) Ephemeral(ctx context.Context, chat string, seconds int64) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	switch seconds {
	case 0, 86400, 604800, 7776000:
	default:
		return errors.New("expiration deve ser 0, 86400 (24h), 604800 (7 dias) ou 7776000 (90 dias)")
	}
	jid, err := s.ResolveJID(ctx, chat)
	if err != nil {
		return err
	}
	return c.SetDisappearingTimer(ctx, jid, time.Duration(seconds)*time.Second, time.Now())
}

// InviteCode tira o código de um link de convite.
func InviteCode(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.LastIndex(v, "/"); i >= 0 {
		v = v[i+1:]
	}
	return v
}

// GroupInviteInfo mostra os dados de um grupo pelo link de convite, sem entrar nele.
func (s *Session) GroupInviteInfo(ctx context.Context, code string) (*types.GroupInfo, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	code = InviteCode(code)
	if code == "" {
		return nil, errors.New("informe o código ou link de convite")
	}
	return c.GetGroupInfoFromLink(ctx, code)
}

// GroupRequests lista os pedidos para entrar no grupo (grupos com aprovação ligada).
func (s *Session) GroupRequests(ctx context.Context, group string) ([]types.GroupParticipantRequest, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return nil, err
	}
	return c.GetGroupRequestParticipants(ctx, jid)
}

// GroupRequestsUpdate aprova ou recusa pedidos para entrar no grupo.
func (s *Session) GroupRequestsUpdate(ctx context.Context, group, action string, members []string) ([]types.GroupParticipant, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	if action != "approve" && action != "reject" {
		return nil, errors.New("action deve ser approve ou reject")
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return nil, err
	}
	ps, err := s.participants(ctx, members)
	if err != nil {
		return nil, err
	}
	return c.UpdateGroupRequestParticipants(ctx, jid, ps, whatsmeow.ParticipantRequestChange(action))
}

// ---------------------------------------------------------------- comunidades

// CommunityCreate cria uma comunidade.
func (s *Session) CommunityCreate(ctx context.Context, name, description string) (*types.GroupInfo, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("informe o nome da comunidade")
	}
	info, err := c.CreateGroup(ctx, whatsmeow.ReqCreateGroup{
		Name:        name,
		GroupParent: types.GroupParent{IsParent: true},
	})
	if err != nil {
		return nil, err
	}
	if description != "" {
		if err := c.SetGroupDescription(ctx, info.JID, description); err == nil {
			info.Topic = description
		}
	}
	return info, nil
}

// CommunityLink liga (ou desliga) um grupo a uma comunidade.
func (s *Session) CommunityLink(ctx context.Context, community, group string, link bool) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	parent, err := s.groupJID(ctx, community)
	if err != nil {
		return err
	}
	child, err := s.groupJID(ctx, group)
	if err != nil {
		return err
	}
	if link {
		return c.LinkGroup(ctx, parent, child)
	}
	return c.UnlinkGroup(ctx, parent, child)
}

// CommunityGroups lista os grupos de uma comunidade.
func (s *Session) CommunityGroups(ctx context.Context, community string) ([]*types.GroupLinkTarget, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := s.groupJID(ctx, community)
	if err != nil {
		return nil, err
	}
	return c.GetSubGroups(ctx, jid)
}

// ---------------------------------------------------------------- perfil e privacidade

// BusinessProfile devolve o perfil comercial (nil quando o número não é comercial).
func (s *Session) BusinessProfile(ctx context.Context, number string) (*types.BusinessProfile, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := s.ResolveJID(ctx, number)
	if err != nil {
		return nil, err
	}
	return c.GetBusinessProfile(ctx, jid)
}

// privacyNames liga os nomes aceitos pela API às configurações do WhatsApp.
var privacyNames = map[string]types.PrivacySettingType{
	"groupadd": types.PrivacySettingTypeGroupAdd, "groupAdd": types.PrivacySettingTypeGroupAdd,
	"last": types.PrivacySettingTypeLastSeen, "lastseen": types.PrivacySettingTypeLastSeen, "lastSeen": types.PrivacySettingTypeLastSeen,
	"status":       types.PrivacySettingTypeStatus,
	"profile":      types.PrivacySettingTypeProfile,
	"readreceipts": types.PrivacySettingTypeReadReceipts, "readReceipts": types.PrivacySettingTypeReadReceipts,
	"online":  types.PrivacySettingTypeOnline,
	"calladd": types.PrivacySettingTypeCallAdd, "callAdd": types.PrivacySettingTypeCallAdd,
}

// SetPrivacy muda as configurações de privacidade informadas e devolve as atuais.
func (s *Session) SetPrivacy(ctx context.Context, values map[string]string) (types.PrivacySettings, error) {
	c, err := s.Client()
	if err != nil {
		return types.PrivacySettings{}, err
	}
	var out types.PrivacySettings
	changed := false
	for k, v := range values {
		t, ok := privacyNames[k]
		if !ok {
			t, ok = privacyNames[strings.ToLower(k)]
		}
		if !ok || strings.TrimSpace(v) == "" {
			continue
		}
		if out, err = c.SetPrivacySetting(ctx, t, types.PrivacySetting(strings.TrimSpace(v))); err != nil {
			return out, fmt.Errorf("%s: %w", k, err)
		}
		changed = true
	}
	if !changed {
		return out, errors.New("informe ao menos uma configuração: groupadd, last, status, profile, readreceipts, online ou calladd")
	}
	return out, nil
}

// RemoveProfilePicture apaga a foto do perfil (ou do grupo informado).
func (s *Session) RemoveProfilePicture(ctx context.Context, group string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	target := types.EmptyJID
	if group != "" {
		if target, err = s.groupJID(ctx, group); err != nil {
			return err
		}
	}
	_, err = c.SetGroupPhoto(ctx, target, nil)
	return err
}

// SetGlobalPresence mostra a instância como online (available) ou offline (unavailable).
func (s *Session) SetGlobalPresence(ctx context.Context, state string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	switch state {
	case "available", "unavailable":
		return c.SendPresence(ctx, types.Presence(state))
	}
	return errors.New("presence deve ser available ou unavailable")
}

// ---------------------------------------------------------------- conversas

// LastMessage identifica a última mensagem de uma conversa. O WhatsApp usa esse dado para
// arquivar ou marcar como não lida no celular e nos outros aparelhos.
type LastMessage struct {
	ID        string
	FromMe    bool
	Sender    string
	Timestamp time.Time
}

// ChatWithLast arquiva, desarquiva ou marca como lida/não lida informando a última mensagem.
func (s *Session) ChatWithLast(ctx context.Context, action, chat string, last *LastMessage) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := s.ResolveJID(ctx, chat)
	if err != nil {
		return err
	}
	if last == nil || last.ID == "" {
		// sem a última mensagem informada, usa a do histórico
		last = s.lastMessage(jid)
	}
	if last == nil || last.ID == "" {
		return s.Chat(ctx, action, chat, 0)
	}
	ts := last.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	key := &waCommon.MessageKey{
		RemoteJID: proto.String(jid.String()),
		FromMe:    proto.Bool(last.FromMe),
		ID:        proto.String(last.ID),
	}
	if last.Sender != "" && jid.Server == types.GroupServer {
		key.Participant = proto.String(last.Sender)
	}
	var patch appstate.PatchInfo
	switch action {
	case "archive", "unarchive":
		patch = appstate.BuildArchive(jid, action == "archive", ts, key)
	case "read", "unread":
		patch = appstate.BuildMarkChatAsRead(jid, action == "read", ts, key)
	default:
		return s.Chat(ctx, action, chat, 0)
	}
	if err := c.SendAppState(ctx, patch); err != nil {
		return err
	}
	s.chatChanged(jid, action)
	return nil
}

// RevokeAs apaga uma mensagem para todos. Com sender vazio apaga uma mensagem da própria
// instância; com sender, um administrador apaga a mensagem de outro membro do grupo.
func (s *Session) RevokeAs(ctx context.Context, chat, id, sender string) (map[string]any, error) {
	if sender == "" {
		return s.Revoke(ctx, chat, id)
	}
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	to, err := s.ResolveJID(ctx, chat)
	if err != nil {
		return nil, err
	}
	from, err := s.ResolveJID(ctx, sender)
	if err != nil {
		return nil, err
	}
	return s.Send(ctx, to, c.BuildRevoke(to, from, types.MessageID(id)), SendOptions{})
}

// ---------------------------------------------------------------- chamadas

// RejectCall recusa uma ligação recebida (os dados vêm no evento CallOffer).
func (s *Session) RejectCall(ctx context.Context, from, callID string) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	if callID == "" {
		return errors.New("informe o callId recebido no evento CallOffer")
	}
	jid, err := s.ResolveJID(ctx, from)
	if err != nil {
		return err
	}
	return c.RejectCall(ctx, jid, callID)
}

// ---------------------------------------------------------------- canais

func newsletterJID(v string) (types.JID, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return types.EmptyJID, errors.New("informe o id do canal")
	}
	if !strings.Contains(v, "@") {
		v += "@" + types.NewsletterServer
	}
	return types.ParseJID(v)
}

// NewsletterCreate cria um canal.
func (s *Session) NewsletterCreate(ctx context.Context, name, description, picture string) (*types.NewsletterMetadata, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("informe o nome do canal")
	}
	params := whatsmeow.CreateNewsletterParams{Name: name, Description: description}
	if picture != "" {
		data, _, err := media.Load(ctx, picture)
		if err != nil {
			return nil, err
		}
		if params.Picture, err = media.ImageJPEG(data); err != nil {
			return nil, err
		}
	}
	return c.CreateNewsletter(ctx, params)
}

// NewsletterInfo devolve os dados de um canal pelo id ou pelo link de convite.
func (s *Session) NewsletterInfo(ctx context.Context, ref string) (*types.NewsletterMetadata, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	if strings.Contains(ref, "whatsapp.com/channel/") || (!strings.Contains(ref, "@") && !strings.HasPrefix(ref, "120363")) {
		return c.GetNewsletterInfoWithInvite(ctx, InviteCode(ref))
	}
	jid, err := newsletterJID(ref)
	if err != nil {
		return nil, err
	}
	return c.GetNewsletterInfo(ctx, jid)
}

// NewsletterFollow segue ou deixa de seguir um canal.
func (s *Session) NewsletterFollow(ctx context.Context, id string, follow bool) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := newsletterJID(id)
	if err != nil {
		return err
	}
	if follow {
		return c.FollowNewsletter(ctx, jid)
	}
	return c.UnfollowNewsletter(ctx, jid)
}

// NewsletterMute silencia ou reativa as notificações de um canal.
func (s *Session) NewsletterMute(ctx context.Context, id string, mute bool) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	jid, err := newsletterJID(id)
	if err != nil {
		return err
	}
	return c.NewsletterToggleMute(ctx, jid, mute)
}

// NewsletterMessages lista as últimas mensagens de um canal.
func (s *Session) NewsletterMessages(ctx context.Context, id string, count int) ([]*types.NewsletterMessage, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := newsletterJID(id)
	if err != nil {
		return nil, err
	}
	if count <= 0 || count > 100 {
		count = 20
	}
	return c.GetNewsletterMessages(ctx, jid, &whatsmeow.GetNewsletterMessagesParams{Count: count})
}
