// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// Membros de grupos: extrair a lista, copiar para outro grupo e enviar mensagem individual a
// cada membro. Copiar e enviar levam minutos (há uma pausa entre cada passo, para o WhatsApp
// não bloquear o número), então rodam como tarefas em segundo plano, com progresso e cancelamento.

// GroupMember é um participante de grupo, já com o número quando o WhatsApp o informa.
type GroupMember struct {
	JID          string `json:"jid"`
	Number       string `json:"number"` // vazio quando o WhatsApp só informa o identificador oculto (LID)
	LID          string `json:"lid,omitempty"`
	Name         string `json:"name,omitempty"`
	IsAdmin      bool   `json:"isAdmin"`
	IsSuperAdmin bool   `json:"isSuperAdmin"`
}

// Limites das tarefas. As pausas mínimas existem para proteger o número de bloqueio.
const (
	GroupJobMaxTargets   = 1000
	groupAddMinDelay     = 5 * time.Second
	groupAddDefaultDelay = 15 * time.Second
	groupSendMinDelay    = 3 * time.Second
	groupSendDefault     = 8 * time.Second
	groupAddMaxBatch     = 10
	groupJobKeep         = 6 * time.Hour
)

// GroupJobItem é o resultado de um membro dentro da tarefa.
type GroupJobItem struct {
	JID    string `json:"jid"`
	Number string `json:"number,omitempty"`
	Status string `json:"status"` // pending, ok, failed, skipped
	Error  string `json:"error,omitempty"`
}

// GroupJob é uma tarefa de membros de grupo em andamento ou terminada.
type GroupJob struct {
	ID         string         `json:"id"`
	InstanceID string         `json:"instanceId"`
	Kind       string         `json:"kind"`   // copy (adicionar em outro grupo) ou send (mensagem a cada membro)
	Status     string         `json:"status"` // running, done, cancelled, failed
	Error      string         `json:"error,omitempty"`
	Source     []string       `json:"source"`
	Target     string         `json:"target,omitempty"`
	Total      int            `json:"total"`
	Processed  int            `json:"processed"`
	OK         int            `json:"ok"`
	Failed     int            `json:"failed"`
	Skipped    int            `json:"skipped"`
	DelaySec   int            `json:"delaySeconds"`
	StartedAt  time.Time      `json:"startedAt"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
	Items      []GroupJobItem `json:"items"`
}

// groupJob é a tarefa viva: os dados mais a trava e o cancelamento.
type groupJob struct {
	GroupJob
	mu     sync.Mutex
	cancel context.CancelFunc
}

var groupJobs sync.Map // id -> *groupJob

func newJobID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// snapshot copia a tarefa para a resposta da API (sem disputar com quem a atualiza).
func (j *groupJob) snapshot(withItems bool) GroupJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := GroupJob{
		ID: j.ID, InstanceID: j.InstanceID, Kind: j.Kind, Status: j.Status, Error: j.Error, Source: j.Source, Target: j.Target,
		Total: j.Total, Processed: j.Processed, OK: j.OK, Failed: j.Failed, Skipped: j.Skipped, DelaySec: j.DelaySec,
		StartedAt: j.StartedAt, FinishedAt: j.FinishedAt, Items: []GroupJobItem{},
	}
	if withItems {
		out.Items = append(out.Items, j.Items...)
	}
	return out
}

func (j *groupJob) mark(i int, status, errText string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if i < 0 || i >= len(j.Items) || j.Items[i].Status != "pending" {
		return
	}
	j.Items[i].Status, j.Items[i].Error = status, errText
	j.Processed++
	switch status {
	case "ok":
		j.OK++
	case "skipped":
		j.Skipped++
	default:
		j.Failed++
	}
}

func (j *groupJob) finish(status, errText string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.Status != "running" {
		return
	}
	now := time.Now()
	j.Status, j.Error, j.FinishedAt = status, errText, &now
}

// GroupJobGet devolve a tarefa da instância.
func (s *Session) GroupJobGet(id string) (GroupJob, bool) {
	v, ok := groupJobs.Load(id)
	if !ok || v.(*groupJob).InstanceID != s.ID {
		return GroupJob{}, false
	}
	return v.(*groupJob).snapshot(true), true
}

// GroupJobList lista as tarefas da instância, da mais nova para a mais antiga (sem os itens).
func (s *Session) GroupJobList() []GroupJob {
	out := []GroupJob{}
	groupJobs.Range(func(k, v any) bool {
		j := v.(*groupJob)
		snap := j.snapshot(false)
		if snap.FinishedAt != nil && time.Since(*snap.FinishedAt) > groupJobKeep {
			groupJobs.Delete(k)
			return true
		}
		if j.InstanceID == s.ID {
			out = append(out, snap)
		}
		return true
	})
	sort.Slice(out, func(a, b int) bool { return out[a].StartedAt.After(out[b].StartedAt) })
	return out
}

// GroupJobCancel interrompe a tarefa. O que já foi feito fica feito.
func (s *Session) GroupJobCancel(id string) (GroupJob, bool) {
	v, ok := groupJobs.Load(id)
	if !ok || v.(*groupJob).InstanceID != s.ID {
		return GroupJob{}, false
	}
	j := v.(*groupJob)
	j.finish("cancelled", "")
	if j.cancel != nil {
		j.cancel()
	}
	return j.snapshot(true), true
}

// runGroupJob percorre os itens em lotes, com pausa entre os lotes. step devolve o resultado de
// cada item do lote (na mesma ordem); um erro de step encerra a tarefa (ex.: número desconectado).
func runGroupJob(ctx context.Context, j *groupJob, batch int, delay time.Duration, step func(ctx context.Context, from, to int) ([]GroupJobItem, error)) {
	if batch < 1 {
		batch = 1
	}
	for from := 0; from < len(j.Items); from += batch {
		if ctx.Err() != nil {
			j.finish("cancelled", "")
			return
		}
		to := min(from+batch, len(j.Items))
		res, err := step(ctx, from, to)
		for i, r := range res {
			if from+i < to {
				j.mark(from+i, r.Status, r.Error)
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				j.finish("cancelled", "")
			} else {
				j.finish("failed", err.Error())
			}
			return
		}
		if to < len(j.Items) {
			select {
			case <-ctx.Done():
				j.finish("cancelled", "")
				return
			case <-time.After(delay):
			}
		}
	}
	j.finish("done", "")
}

// clampDelay aplica o padrão e o mínimo à pausa pedida (em segundos).
func clampDelay(seconds int, def, minimum time.Duration) time.Duration {
	if seconds <= 0 {
		return def
	}
	d := time.Duration(seconds) * time.Second
	if d < minimum {
		return minimum
	}
	if d > 10*time.Minute {
		return 10 * time.Minute
	}
	return d
}

// sameUser compara dois participantes pelo número ou pelo LID.
func sameUser(a, b types.GroupParticipant) bool {
	if !a.PhoneNumber.IsEmpty() && !b.PhoneNumber.IsEmpty() {
		return a.PhoneNumber.User == b.PhoneNumber.User
	}
	if !a.LID.IsEmpty() && !b.LID.IsEmpty() {
		return a.LID.User == b.LID.User
	}
	return a.JID.User == b.JID.User && a.JID.Server == b.JID.Server
}

// pickMembers escolhe os membros de src para a tarefa: tira o próprio número, quem já está em
// exclude (ex.: o grupo de destino), os administradores (se pedido) e os repetidos.
func pickMembers(src, exclude []types.GroupParticipant, self types.JID, selfLID types.JID, skipAdmins bool, limit int) []types.GroupParticipant {
	out := []types.GroupParticipant{}
	me := types.GroupParticipant{JID: self.ToNonAD(), PhoneNumber: self.ToNonAD(), LID: selfLID.ToNonAD()}
next:
	for _, p := range src {
		if sameUser(p, me) || p.JID.User == me.JID.User || (!me.LID.IsEmpty() && p.JID.User == me.LID.User) {
			continue
		}
		if skipAdmins && (p.IsAdmin || p.IsSuperAdmin) {
			continue
		}
		for _, e := range exclude {
			if sameUser(p, e) {
				continue next
			}
		}
		for _, o := range out {
			if sameUser(p, o) {
				continue next
			}
		}
		out = append(out, p)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Session) memberNumber(ctx context.Context, c *whatsmeow.Client, p types.GroupParticipant) string {
	if !p.PhoneNumber.IsEmpty() {
		return p.PhoneNumber.User
	}
	if p.JID.Server == types.DefaultUserServer {
		return p.JID.User
	}
	lid := p.LID
	if lid.IsEmpty() && p.JID.Server == types.HiddenUserServer {
		lid = p.JID
	}
	if !lid.IsEmpty() && c != nil && c.Store != nil && c.Store.LIDs != nil {
		if pn, err := c.Store.LIDs.GetPNForLID(ctx, lid); err == nil && !pn.IsEmpty() {
			return pn.User
		}
	}
	return ""
}

func (s *Session) member(ctx context.Context, c *whatsmeow.Client, p types.GroupParticipant) GroupMember {
	m := GroupMember{JID: p.JID.ToNonAD().String(), Number: s.memberNumber(ctx, c, p), Name: p.DisplayName, IsAdmin: p.IsAdmin, IsSuperAdmin: p.IsSuperAdmin}
	if !p.LID.IsEmpty() {
		m.LID = p.LID.ToNonAD().String()
	}
	return m
}

// GroupMembers extrai os membros de um grupo: número (quando informado), JID e se é administrador.
func (s *Session) GroupMembers(ctx context.Context, group string) (map[string]any, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	jid, err := s.groupJID(ctx, group)
	if err != nil {
		return nil, err
	}
	info, err := c.GetGroupInfo(ctx, jid)
	if err != nil {
		return nil, err
	}
	members := make([]GroupMember, 0, len(info.Participants))
	withNumber := 0
	for _, p := range info.Participants {
		m := s.member(ctx, c, p)
		if m.Number != "" {
			withNumber++
		}
		members = append(members, m)
	}
	return map[string]any{"groupJid": jid.String(), "name": info.Name, "total": len(members), "withNumber": withNumber, "members": members}, nil
}

func selfJIDs(c *whatsmeow.Client) (types.JID, types.JID) {
	if c == nil || c.Store == nil || c.Store.ID == nil {
		return types.EmptyJID, types.EmptyJID
	}
	return c.Store.ID.ToNonAD(), c.Store.LID.ToNonAD()
}

func (s *Session) startGroupJob(j *groupJob, batch int, delay time.Duration, step func(ctx context.Context, from, to int) ([]GroupJobItem, error)) GroupJob {
	ctx, cancel := context.WithCancel(context.Background())
	j.ID, j.InstanceID, j.Status, j.StartedAt, j.cancel = newJobID(), s.ID, "running", time.Now(), cancel
	j.Total, j.DelaySec = len(j.Items), int(delay/time.Second)
	groupJobs.Store(j.ID, j)
	s.logf("INFO", "tarefa de grupo %s (%s) iniciada: %d membros, pausa de %s", j.ID, j.Kind, j.Total, delay)
	go func() {
		defer cancel()
		runGroupJob(ctx, j, batch, delay, step)
		snap := j.snapshot(false)
		s.logf("INFO", "tarefa de grupo %s terminou (%s): %d ok, %d falhas, %d pulados", j.ID, snap.Status, snap.OK, snap.Failed, snap.Skipped)
		s.emit("GroupJob", snap)
	}()
	return j.snapshot(true)
}

// addErrorText explica o código que o WhatsApp devolve ao adicionar alguém num grupo.
func addErrorText(code int) string {
	switch code {
	case 0:
		return ""
	case 403:
		return "a pessoa só aceita entrar em grupos por convite (privacidade)"
	case 408:
		return "a pessoa saiu do grupo há pouco tempo e ainda não pode ser adicionada"
	case 409:
		return "já está no grupo"
	case 401:
		return "a pessoa bloqueou este número"
	case 404:
		return "número sem WhatsApp"
	default:
		return fmt.Sprintf("o WhatsApp recusou (código %d)", code)
	}
}

// GroupCopyOptions controla a cópia de membros de um grupo para outro.
type GroupCopyOptions struct {
	From       string
	To         string
	SkipAdmins bool
	Batch      int // quantos por vez (1 a 10)
	Delay      int // segundos entre os lotes (mínimo 5)
	Limit      int
}

// GroupCopyMembers adiciona no grupo de destino os membros do grupo de origem que ainda não estão
// nele. O número precisa ser administrador do destino. Roda em segundo plano.
func (s *Session) GroupCopyMembers(ctx context.Context, opt GroupCopyOptions) (GroupJob, error) {
	c, err := s.Client()
	if err != nil {
		return GroupJob{}, err
	}
	from, err := s.groupJID(ctx, opt.From)
	if err != nil {
		return GroupJob{}, errors.New("informe o grupo de origem (fromGroupJid)")
	}
	to, err := s.groupJID(ctx, opt.To)
	if err != nil {
		return GroupJob{}, errors.New("informe o grupo de destino (toGroupJid)")
	}
	if from == to {
		return GroupJob{}, errors.New("o grupo de origem e o de destino são o mesmo")
	}
	src, err := c.GetGroupInfo(ctx, from)
	if err != nil {
		return GroupJob{}, fmt.Errorf("grupo de origem: %w", err)
	}
	dst, err := c.GetGroupInfo(ctx, to)
	if err != nil {
		return GroupJob{}, fmt.Errorf("grupo de destino: %w", err)
	}
	self, selfLID := selfJIDs(c)
	limit := opt.Limit
	if limit <= 0 || limit > GroupJobMaxTargets {
		limit = GroupJobMaxTargets
	}
	targets := pickMembers(src.Participants, dst.Participants, self, selfLID, opt.SkipAdmins, limit)
	if len(targets) == 0 {
		return GroupJob{}, errors.New("todos os membros do grupo de origem já estão no grupo de destino")
	}
	job := &groupJob{GroupJob: GroupJob{Kind: "copy", Source: []string{from.String()}, Target: to.String()}}
	jids := make([]types.JID, len(targets))
	for i, p := range targets {
		jids[i] = p.JID.ToNonAD()
		job.Items = append(job.Items, GroupJobItem{JID: jids[i].String(), Number: s.memberNumber(ctx, c, p), Status: "pending"})
	}
	batch := opt.Batch
	if batch < 1 {
		batch = 5
	}
	batch = min(batch, groupAddMaxBatch)
	step := func(ctx context.Context, a, b int) ([]GroupJobItem, error) {
		cli, err := s.Client()
		if err != nil {
			return nil, err
		}
		res, err := cli.UpdateGroupParticipants(ctx, to, jids[a:b], whatsmeow.ParticipantChangeAdd)
		out := make([]GroupJobItem, b-a)
		if err != nil {
			// O lote inteiro falhou (ex.: o número não é administrador do destino): não adianta continuar
			for i := range out {
				out[i] = GroupJobItem{Status: "failed", Error: err.Error()}
			}
			return out, fmt.Errorf("não foi possível adicionar no grupo de destino: %w", err)
		}
		for i := range out {
			out[i] = GroupJobItem{Status: "ok"}
			for _, r := range res {
				if r.JID.User == jids[a+i].User || r.PhoneNumber.User == jids[a+i].User || r.LID.User == jids[a+i].User {
					if r.Error == 409 {
						out[i] = GroupJobItem{Status: "skipped", Error: addErrorText(409)}
					} else if r.Error != 0 {
						out[i] = GroupJobItem{Status: "failed", Error: addErrorText(r.Error)}
					}
					break
				}
			}
		}
		return out, nil
	}
	return s.startGroupJob(job, batch, clampDelay(opt.Delay, groupAddDefaultDelay, groupAddMinDelay), step), nil
}

// GroupSendOptions controla o envio individual aos membros de um ou mais grupos.
type GroupSendOptions struct {
	Groups     []string
	SkipAdmins bool
	Delay      int // segundos entre as mensagens (mínimo 3)
	Limit      int
}

// GroupSendToMembers envia a mensagem no privado de cada membro dos grupos (não no grupo).
// Membros repetidos em mais de um grupo recebem uma vez só. Roda em segundo plano.
func (s *Session) GroupSendToMembers(ctx context.Context, opt GroupSendOptions, msg *waE2E.Message) (GroupJob, error) {
	c, err := s.Client()
	if err != nil {
		return GroupJob{}, err
	}
	if msg == nil {
		return GroupJob{}, errors.New("informe o texto ou a mídia")
	}
	if len(opt.Groups) == 0 {
		return GroupJob{}, errors.New("informe o grupo (groupJid ou groupJids)")
	}
	job := &groupJob{GroupJob: GroupJob{Kind: "send"}}
	all := []types.GroupParticipant{}
	for _, g := range opt.Groups {
		jid, err := s.groupJID(ctx, strings.TrimSpace(g))
		if err != nil {
			return GroupJob{}, err
		}
		info, err := c.GetGroupInfo(ctx, jid)
		if err != nil {
			return GroupJob{}, fmt.Errorf("grupo %s: %w", g, err)
		}
		job.Source = append(job.Source, jid.String())
		all = append(all, info.Participants...)
	}
	self, selfLID := selfJIDs(c)
	limit := opt.Limit
	if limit <= 0 || limit > GroupJobMaxTargets {
		limit = GroupJobMaxTargets
	}
	targets := pickMembers(all, nil, self, selfLID, opt.SkipAdmins, limit)
	if len(targets) == 0 {
		return GroupJob{}, errors.New("nenhum membro para enviar")
	}
	jids := make([]types.JID, len(targets))
	for i, p := range targets {
		num := s.memberNumber(ctx, c, p)
		if num != "" {
			jids[i] = types.NewJID(num, types.DefaultUserServer)
		} else {
			jids[i] = p.JID.ToNonAD()
		}
		job.Items = append(job.Items, GroupJobItem{JID: jids[i].String(), Number: num, Status: "pending"})
	}
	delay := clampDelay(opt.Delay, groupSendDefault, groupSendMinDelay)
	step := func(ctx context.Context, a, b int) ([]GroupJobItem, error) {
		if _, err := s.Client(); err != nil {
			return nil, err // desconectou: para a tarefa em vez de falhar em todos
		}
		if _, err := s.Send(ctx, jids[a], proto.Clone(msg).(*waE2E.Message), SendOptions{}); err != nil {
			return []GroupJobItem{{Status: "failed", Error: err.Error()}}, nil
		}
		return []GroupJobItem{{Status: "ok"}}, nil
	}
	return s.startGroupJob(job, 1, delay, step), nil
}
