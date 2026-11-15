// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// Ferramentas de extração e de grupos: membros de vários grupos numa lista só, membros ativos,
// contatos das conversas, filtro de números com WhatsApp, conferência de links de convite e
// adição de participantes com resposta por pessoa.

// Member é um participante extraído de um ou mais grupos.
type Member struct {
	Phone  string   `json:"phone"`
	JID    string   `json:"jid"`
	Name   string   `json:"name,omitempty"`
	Admin  bool     `json:"admin"`
	Groups []string `json:"groups"`
}

// MemberGroup resume um grupo lido na extração.
type MemberGroup struct {
	JID         string `json:"jid"`
	Name        string `json:"name"`
	Total       int    `json:"total"`
	Hidden      int    `json:"hidden"` // participantes com o número oculto pelo WhatsApp
	IsCommunity bool   `json:"isCommunity"`
	Error       string `json:"error,omitempty"`
}

// MembersResult é a resposta da extração de membros.
type MembersResult struct {
	Groups  []MemberGroup `json:"groups"`
	Members []Member      `json:"members"`
	Total   int           `json:"total"`
	Hidden  int           `json:"hidden"`
}

// MembersOptions filtra a extração.
type MembersOptions struct {
	OnlyAdmins    bool
	ExcludeAdmins bool
}

// participantPhone devolve o telefone do participante, ou vazio quando o WhatsApp só mostra o
// identificador oculto (@lid).
func participantPhone(p types.GroupParticipant) types.JID {
	if p.PhoneNumber.Server == types.DefaultUserServer {
		return p.PhoneNumber.ToNonAD()
	}
	if p.JID.Server == types.DefaultUserServer {
		return p.JID.ToNonAD()
	}
	return types.EmptyJID
}

// MergeMembers junta os participantes de vários grupos numa lista sem repetição. own é o número
// da própria instância (fica de fora); name devolve o nome conhecido de um contato.
func MergeMembers(infos []*types.GroupInfo, own string, name func(types.JID) string, opt MembersOptions) *MembersResult {
	res := &MembersResult{Groups: []MemberGroup{}, Members: []Member{}}
	index := map[string]int{}
	for _, g := range infos {
		if g == nil {
			continue
		}
		sum := MemberGroup{JID: g.JID.String(), Name: g.Name, Total: len(g.Participants), IsCommunity: g.IsParent}
		for _, p := range g.Participants {
			admin := p.IsAdmin || p.IsSuperAdmin
			if (opt.OnlyAdmins && !admin) || (opt.ExcludeAdmins && admin) {
				continue
			}
			jid := participantPhone(p)
			if jid.IsEmpty() {
				sum.Hidden++
				continue
			}
			if jid.User == own {
				continue
			}
			i, seen := index[jid.User]
			if !seen {
				m := Member{Phone: jid.User, JID: jid.String(), Groups: []string{}}
				if name != nil {
					m.Name = name(jid)
				}
				res.Members = append(res.Members, m)
				i = len(res.Members) - 1
				index[jid.User] = i
			}
			res.Members[i].Admin = res.Members[i].Admin || admin
			res.Members[i].Groups = append(res.Members[i].Groups, g.Name)
		}
		res.Hidden += sum.Hidden
		res.Groups = append(res.Groups, sum)
	}
	sort.SliceStable(res.Members, func(a, b int) bool { return res.Members[a].Phone < res.Members[b].Phone })
	res.Total = len(res.Members)
	return res
}

// contactName devolve o nome salvo na agenda ou o nome de perfil de um contato.
func (s *Session) contactName(ctx context.Context, c *whatsmeow.Client) func(types.JID) string {
	return func(j types.JID) string {
		ci, err := c.Store.Contacts.GetContact(ctx, j)
		if err != nil || !ci.Found {
			return ""
		}
		for _, v := range []string{ci.FullName, ci.FirstName, ci.BusinessName, ci.PushName} {
			if v != "" {
				return v
			}
		}
		return ""
	}
}

// GroupMembers extrai os membros dos grupos informados (todos os grupos da instância quando a
// lista vem vazia), sem repetir quem está em mais de um.
func (s *Session) GroupMembers(ctx context.Context, groups []string, opt MembersOptions) (*MembersResult, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	var infos []*types.GroupInfo
	var failed []MemberGroup
	if len(groups) == 0 {
		if infos, err = c.GetJoinedGroups(ctx); err != nil {
			return nil, err
		}
	} else {
		if len(groups) > 100 {
			return nil, errors.New("no máximo 100 grupos por extração")
		}
		for _, g := range groups {
			jid, err := s.groupJID(ctx, g)
			if err != nil {
				failed = append(failed, MemberGroup{JID: g, Error: err.Error()})
				continue
			}
			info, err := c.GetGroupInfo(ctx, jid)
			if err != nil {
				failed = append(failed, MemberGroup{JID: jid.String(), Error: err.Error()})
				continue
			}
			infos = append(infos, info)
		}
	}
	own := ""
	if c.Store.ID != nil {
		own = c.Store.ID.User
	}
	res := MergeMembers(infos, own, s.contactName(ctx, c), opt)
	res.Groups = append(res.Groups, failed...)
	return res, nil
}

// ActiveMember é quem escreveu num grupo no período.
type ActiveMember struct {
	Phone    string `json:"phone"`
	JID      string `json:"jid"`
	Name     string `json:"name,omitempty"`
	Messages int64  `json:"messages"`
	LastAt   int64  `json:"lastAt"`
}

// ActiveMembers lista quem mandou mensagem no grupo nos últimos dias, do mais ativo para o menos.
// Usa o histórico guardado pela BOB (STORE_MESSAGES ligado).
func (s *Session) ActiveMembers(ctx context.Context, group string, days int) ([]ActiveMember, error) {
	if !s.storing() {
		return nil, errors.New("o histórico de mensagens está desligado (STORE_MESSAGES)")
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return nil, err
	}
	if days <= 0 || days > 365 {
		days = 30
	}
	rows, err := s.e.Repo.ActiveSenders(s.ID, jid.String(), time.Now().AddDate(0, 0, -days).Unix())
	if err != nil {
		return nil, err
	}
	out := make([]ActiveMember, 0, len(rows))
	for _, r := range rows {
		j, err := types.ParseJID(r.Sender)
		if err != nil || j.Server != types.DefaultUserServer {
			continue
		}
		out = append(out, ActiveMember{Phone: j.User, JID: j.ToNonAD().String(), Name: r.PushName, Messages: r.Messages, LastAt: r.LastAt})
	}
	return out, nil
}

// ChatContacts lista as pessoas com quem a instância tem conversa (não inclui grupos).
func (s *Session) ChatContacts() ([]map[string]any, error) {
	out := []map[string]any{}
	for offset := 0; ; offset += 500 {
		chats, _, err := s.e.Repo.Chats(s.ID, 500, offset)
		if err != nil {
			return nil, err
		}
		for _, ch := range chats {
			j, err := types.ParseJID(ch.JID)
			if err != nil || j.Server != types.DefaultUserServer {
				continue
			}
			out = append(out, map[string]any{"phone": j.User, "jid": ch.JID, "name": ch.Name})
		}
		if len(chats) < 500 {
			return out, nil
		}
	}
}

// NumberFilter separa os números que têm WhatsApp dos que não têm.
type NumberFilter struct {
	Valid   []map[string]any `json:"valid"`
	Invalid []string         `json:"invalid"`
}

// FilterNumbers confere uma lista grande de números, em lotes, e separa os que têm WhatsApp.
func (s *Session) FilterNumbers(ctx context.Context, numbers []string) (*NumberFilter, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var q []string
	for _, n := range numbers {
		if d := digits(n); len(d) >= 8 && len(d) <= 15 && !seen[d] {
			seen[d] = true
			q = append(q, d)
		}
	}
	if len(q) == 0 {
		return nil, errors.New("informe os números")
	}
	if len(q) > 5000 {
		return nil, errors.New("no máximo 5000 números por vez")
	}
	res := &NumberFilter{Valid: []map[string]any{}, Invalid: []string{}}
	for i := 0; i < len(q); i += 50 {
		batch := q[i:min(i+50, len(q))]
		plus := make([]string, len(batch))
		for k, d := range batch {
			plus[k] = "+" + d
		}
		rows, err := c.IsOnWhatsApp(ctx, plus)
		if err != nil {
			return nil, err
		}
		found := map[string]bool{}
		for _, r := range rows {
			d := digits(r.Query)
			found[d] = true
			if r.IsIn {
				res.Valid = append(res.Valid, map[string]any{"number": d, "jid": r.JID.String()})
			} else {
				res.Invalid = append(res.Invalid, d)
			}
		}
		for _, d := range batch {
			if !found[d] {
				res.Invalid = append(res.Invalid, d)
			}
		}
		if i+50 < len(q) {
			if err := pause(ctx, time.Second); err != nil {
				return nil, err
			}
		}
	}
	return res, nil
}

// CheckInvites confere vários links de convite sem entrar nos grupos.
func (s *Session) CheckInvites(ctx context.Context, links []string) ([]map[string]any, error) {
	if len(links) == 0 {
		return nil, errors.New("informe os links")
	}
	if len(links) > 50 {
		return nil, errors.New("no máximo 50 links por vez")
	}
	out := make([]map[string]any, 0, len(links))
	for i, l := range links {
		row := map[string]any{"link": l, "valid": false}
		g, err := s.GroupInviteInfo(ctx, l)
		if err != nil {
			if errors.Is(err, ErrNotLogged) || errors.Is(err, ErrNotConnected) {
				return nil, err
			}
			row["error"] = err.Error()
		} else {
			row["valid"] = true
			row["jid"] = g.JID.String()
			row["name"] = g.Name
			row["size"] = len(g.Participants)
			row["isCommunity"] = g.IsParent
			row["joinApproval"] = g.IsJoinApprovalRequired
		}
		out = append(out, row)
		if i+1 < len(links) {
			if err := pause(ctx, 1500*time.Millisecond); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// AddResult é a resposta do WhatsApp para cada pessoa adicionada.
type AddResult struct {
	Number string `json:"number"`
	Status string `json:"status"` // added, member, invited, refused, failed
	Code   int    `json:"code,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// AddOptions controla a adição de participantes.
type AddOptions struct {
	// InviteFallback manda o link de convite para quem não aceita ser adicionado por desconhecidos.
	InviteFallback bool
	InviteText     string
}

// addDetail explica o código que o WhatsApp devolve por participante.
func addDetail(code int) string {
	switch code {
	case 401:
		return "a pessoa bloqueou este número"
	case 403:
		return "a pessoa não aceita ser adicionada a grupos por quem não está na agenda dela"
	case 404:
		return "o número não tem WhatsApp"
	case 408:
		return "a pessoa saiu do grupo há pouco tempo e ainda não pode ser adicionada de novo"
	case 409:
		return "já está no grupo"
	}
	return "o WhatsApp recusou a adição"
}

// AddParticipant adiciona uma pessoa ao grupo ou comunidade e diz o que aconteceu com ela.
func (s *Session) AddParticipant(ctx context.Context, group, number string, opt AddOptions) (AddResult, error) {
	res := AddResult{Number: digits(number), Status: "failed"}
	c, err := s.Client()
	if err != nil {
		return res, err
	}
	gjid, err := s.groupJID(ctx, group)
	if err != nil {
		return res, err
	}
	jid, err := s.ResolveJID(ctx, number)
	if err != nil {
		res.Detail = err.Error()
		return res, nil
	}
	rows, err := c.UpdateGroupParticipants(ctx, gjid, []types.JID{jid}, whatsmeow.ParticipantChangeAdd)
	if err != nil {
		return res, err
	}
	if len(rows) > 0 {
		res.Code = rows[0].Error
	}
	switch res.Code {
	case 0:
		res.Status = "added"
	case 409:
		res.Status, res.Detail = "member", addDetail(409)
	case 403:
		res.Status, res.Detail = "refused", addDetail(403)
		if opt.InviteFallback {
			link, err := c.GetGroupInviteLink(ctx, gjid, false)
			if err != nil {
				res.Detail += "; não foi possível gerar o link de convite: " + err.Error()
				break
			}
			text := strings.TrimSpace(opt.InviteText)
			if text == "" {
				text = "Olá! Este é o convite para entrar no grupo:"
			}
			if _, err := s.Send(ctx, jid, Text(text+"\n"+link), SendOptions{}); err != nil {
				res.Detail += "; o convite não foi enviado: " + err.Error()
				break
			}
			res.Status, res.Detail = "invited", "recebeu o link de convite"
		}
	default:
		res.Detail = addDetail(res.Code)
	}
	return res, nil
}

// pause espera d ou até o contexto acabar.
func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Personalize troca {{numero}} pelo número de destino nos envios em massa.
func Personalize(text, number string) string {
	return strings.ReplaceAll(text, "{{numero}}", digits(number))
}
