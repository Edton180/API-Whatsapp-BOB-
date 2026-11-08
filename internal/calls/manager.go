// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package calls liga as chamadas de voz e vídeo do WhatsApp ao navegador dos atendentes.
//
// O motor de chamadas é a biblioteca meowcaller (MIT), que fala o protocolo de chamadas do
// WhatsApp sobre a whatsmeow. A ponte com o navegador usa WebRTC (Pion, MIT): o áudio vai em
// G.722 (ou PCMU) e o vídeo H.264 passa sem ser recodificado. Nada de áudio ou vídeo é gravado:
// a BOB guarda só os dados da chamada (quem, quando, duração e como terminou).
package calls

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/purpshell/meowcaller"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"github.com/edton180/api-whatsapp-bob/internal/config"
	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

// Erros devolvidos às rotas.
var (
	ErrDisabled = errors.New("chamadas desligadas nesta instância: ligue em /call/settings")
	ErrNotLive  = errors.New("a chamada não está mais em andamento")
	ErrTaken    = errors.New("a chamada já foi atendida por outro atendente")
	ErrNotYours = errors.New("a chamada está com outro atendente")
	ErrNoMedia  = errors.New("a chamada ainda não tem um navegador ligado")
)

// Eventos da instância (categoria CALL no webhook, no WebSocket e no padrão v2).
const (
	EventRinging   = "CallRinging"   // chamada nova tocando (recebida ou feita)
	EventConnected = "CallConnected" // atendida, áudio passando
	EventEnded     = "CallEnded"     // encerrada, recusada ou perdida
	EventMedia     = "CallMedia"     // câmera, microfone ou atendente mudou
)

// Manager guarda o motor de chamadas de cada instância e as chamadas em andamento.
type Manager struct {
	Engine *engine.Engine
	Repo   *store.Repo
	Cfg    config.WebRTC

	api      *webrtc.API
	closeAPI func() error

	mu      sync.Mutex
	clients map[string]*meowcaller.Client
	live    map[string]*liveCall
}

// New prepara o gerenciador e encerra no banco as chamadas que ficaram abertas (a BOB caiu
// no meio delas).
func New(eng *engine.Engine, repo *store.Repo, cfg config.WebRTC) (*Manager, error) {
	api, closer, err := newAPI(cfg)
	if err != nil {
		return nil, err
	}
	m := &Manager{Engine: eng, Repo: repo, Cfg: cfg, api: api, closeAPI: closer,
		clients: map[string]*meowcaller.Client{}, live: map[string]*liveCall{}}
	if _, err := repo.CloseOpenCalls(time.Now()); err != nil {
		return nil, fmt.Errorf("chamadas abertas: %w", err)
	}
	return m, nil
}

// Close encerra todas as chamadas e libera a porta UDP.
func (m *Manager) Close() error {
	m.mu.Lock()
	list := make([]*liveCall, 0, len(m.live))
	for _, lc := range m.live {
		list = append(list, lc)
	}
	m.mu.Unlock()
	for _, lc := range list {
		_ = lc.call.Hangup()
	}
	return m.closeAPI()
}

func key(inst, callID string) string { return inst + "/" + strings.ToUpper(callID) }

// ---------------------------------------------------------------- engine.CallHooks

// Attach instala o motor de chamadas no cliente novo da instância, antes do Connect. Só
// instâncias com chamadas ligadas recebem o motor: ele pré-aceita toda chamada recebida.
func (m *Manager) Attach(s *engine.Session, cli *whatsmeow.Client) {
	inst := s.Instance()
	m.mu.Lock()
	delete(m.clients, inst.Id)
	m.mu.Unlock()
	if !inst.CallsEnabled {
		return
	}
	mc := meowcaller.NewClient(cli)
	mc.OnIncomingCall(func(c *meowcaller.Call) { m.onIncoming(s, cli, c) })
	m.mu.Lock()
	m.clients[inst.Id] = mc
	m.mu.Unlock()
	s.Logf("INFO", "chamadas de voz e vídeo ligadas")
}

// Handles diz se a instância tem o motor de chamadas instalado.
func (m *Manager) Handles(instanceID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.clients[instanceID] != nil
}

// Detach encerra as chamadas da instância e tira o motor.
func (m *Manager) Detach(instanceID string) {
	m.mu.Lock()
	delete(m.clients, instanceID)
	var list []*liveCall
	for _, lc := range m.live {
		if lc.inst == instanceID {
			list = append(list, lc)
		}
	}
	m.mu.Unlock()
	for _, lc := range list {
		_ = lc.call.Hangup()
	}
}

// ---------------------------------------------------------------- chamada em andamento

// liveCall é uma chamada viva: o lado do WhatsApp (meowcaller) e o navegador (bridge).
type liveCall struct {
	m      *Manager
	sess   *engine.Session
	call   *meowcaller.Call
	inst   string
	id     string
	remote string
	fromMe bool
	video  bool

	src  *liveSource
	sink *browserSink

	mu        sync.Mutex
	br        *bridge
	agent     string
	muted     bool
	camera    bool
	upgrade   bool // o contato pediu para ligar o vídeo e ainda não foi aceito
	claimed   bool // um atendente já pegou a chamada (atendendo ou atendida)
	connected bool
	ended     bool
}

// track liga os dois lados da chamada. remote vazio: o número sai do LID da chamada.
func (m *Manager) track(s *engine.Session, cli *whatsmeow.Client, c *meowcaller.Call, fromMe, video bool, remote string) *liveCall {
	if remote == "" {
		remote = remoteJID(cli, c.Peer())
	}
	lc := &liveCall{m: m, sess: s, call: c, inst: s.ID, id: c.ID(), fromMe: fromMe, video: video,
		remote: remote, src: &liveSource{}, claimed: fromMe, camera: fromMe && video}
	lc.sink = &browserSink{codec: newAudioCodec(webrtc.MimeTypeG722)}
	c.Play(lc.src)
	c.Receive(lc.sink)
	c.ReceiveVideo(meowcaller.VideoSinkFunc(lc.videoFromWhatsApp))
	c.OnVideoKeyframeRequest(lc.keyframe)
	c.OnEnd(lc.onEnd)
	c.OnVideoState(lc.onVideoState)
	c.OnMuteState(func(muted bool) { lc.emitMedia(map[string]any{"peerMuted": muted}) })
	if fromMe {
		c.OnPeerAccept(lc.onAnswered)
	}
	m.mu.Lock()
	m.live[key(lc.inst, lc.id)] = lc
	m.mu.Unlock()
	return lc
}

// remoteJID troca o LID da chamada pelo número (PN) quando a whatsmeow conhece os dois.
func remoteJID(cli *whatsmeow.Client, peer types.JID) string {
	if peer.Server == types.HiddenUserServer && cli != nil && cli.Store != nil && cli.Store.LIDs != nil {
		if pn, err := cli.Store.LIDs.GetPNForLID(context.Background(), peer); err == nil && !pn.IsEmpty() {
			return pn.ToNonAD().String()
		}
	}
	return peer.ToNonAD().String()
}

func (m *Manager) onIncoming(s *engine.Session, cli *whatsmeow.Client, c *meowcaller.Call) {
	if _, group := c.GroupState(); group {
		// Chamadas em grupo ficam para o celular: a ponte é de uma pessoa para uma.
		return
	}
	inst := s.Instance()
	lc := m.track(s, cli, c, false, c.IsVideo(), "")
	rec, err := m.Repo.CreateCall(&store.Call{InstanceID: lc.inst, CallID: lc.id, RemoteJid: lc.remote, IsVideo: lc.video})
	if err != nil {
		s.Logf("ERROR", "não consegui registrar a chamada %s: %v", lc.id, err)
	}
	if inst.RejectCall {
		_ = c.Reject()
		if msg := strings.TrimSpace(inst.MsgRejectCall); msg != "" {
			if to, err := types.ParseJID(lc.remote); err == nil {
				go func() {
					_, _ = s.Send(context.Background(), to, &waE2E.Message{Conversation: &msg}, engine.SendOptions{})
				}()
			}
		}
		s.Logf("INFO", "chamada de %s recusada (rejectCall)", lc.remote)
		return
	}
	s.Logf("INFO", "chamada %s de %s tocando", map[bool]string{true: "de vídeo", false: "de voz"}[lc.video], lc.remote)
	if rec != nil {
		s.Emit(EventRinging, callJSON(rec, lc))
	}
}

func (lc *liveCall) onAnswered() {
	lc.mu.Lock()
	first := !lc.connected
	lc.connected = true
	lc.mu.Unlock()
	if !first {
		return
	}
	if rec, err := lc.m.Repo.MarkCallAnswered(lc.inst, lc.id, time.Now()); err == nil {
		lc.sess.Emit(EventConnected, callJSON(rec, lc))
	}
}

func (lc *liveCall) onEnd(reason string) {
	lc.mu.Lock()
	if lc.ended {
		lc.mu.Unlock()
		return
	}
	lc.ended = true
	br := lc.br
	lc.br = nil
	lc.mu.Unlock()
	if br != nil {
		br.Close()
	}
	_ = lc.src.Close()
	_ = lc.sink.Close()
	lc.m.mu.Lock()
	delete(lc.m.live, key(lc.inst, lc.id))
	lc.m.mu.Unlock()

	status := store.CallEnded
	if reason == "rejected" || strings.Contains(reason, "reject") || reason == "busy" {
		status = store.CallRejected
	}
	if strings.Contains(reason, "elsewhere") {
		reason = store.ReasonElsewhere
	}
	rec, err := lc.m.Repo.FinishCall(lc.inst, lc.id, status, reason, time.Now())
	if err != nil {
		lc.sess.Logf("ERROR", "não consegui encerrar a chamada %s: %v", lc.id, err)
		return
	}
	lc.sess.Logf("INFO", "chamada %s encerrada (%s, %s)", lc.id, rec.Status, reason)
	lc.sess.Emit(EventEnded, callJSON(rec, lc))
}

func (lc *liveCall) bridgeRef() *bridge {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return lc.br
}

func (lc *liveCall) videoFromWhatsApp(au []byte) {
	if br := lc.bridgeRef(); br != nil {
		_ = br.WriteVideo(au)
	}
}

func (lc *liveCall) keyframe() {
	if br := lc.bridgeRef(); br != nil {
		br.RequestKeyframe()
	}
}

func (lc *liveCall) videoFromBrowser(au []byte) {
	lc.mu.Lock()
	on := lc.camera
	lc.mu.Unlock()
	if on {
		_ = lc.call.SendVideo(au)
	}
}

func (lc *liveCall) onVideoState(v meowcaller.VideoState) {
	lc.mu.Lock()
	if v.Upgrade {
		lc.upgrade = true
	}
	if v.Active {
		lc.video = true
	}
	lc.mu.Unlock()
	lc.emitMedia(map[string]any{"peerVideo": v.Active, "videoRequest": v.Upgrade, "orientation": v.Orientation})
}

func (lc *liveCall) emitMedia(extra map[string]any) {
	rec, err := lc.m.Repo.CallByID(lc.inst, lc.id)
	if err != nil {
		return
	}
	data := callJSON(rec, lc)
	for k, v := range extra {
		data[k] = v
	}
	lc.sess.Emit(EventMedia, data)
}

// attach liga o navegador do atendente à chamada (atender, ligar ou transferir).
func (lc *liveCall) attach(agent, offer string) (string, error) {
	servers := ICEServers(lc.m.Cfg, agent, time.Now())
	var br *bridge
	br, answer, err := newBridge(lc.m.api, servers, offer, bridgeHooks{
		audio: lc.src.push,
		video: lc.videoFromBrowser,
		connected: func() {
			lc.sess.Logf("INFO", "navegador de %s ligado à chamada %s", agentName(agent), lc.id)
		},
		failed: func() {
			// O navegador caiu ou a rede não deixou conectar: a chamada não fica muda para o contato.
			if lc.bridgeRef() == br {
				lc.sess.Logf("WARN", "conexão com o navegador falhou na chamada %s", lc.id)
				_ = lc.call.Hangup()
			}
		},
	})
	if err != nil {
		return "", err
	}
	lc.mu.Lock()
	if lc.ended {
		lc.mu.Unlock()
		br.Close()
		return "", ErrNotLive
	}
	old := lc.br
	lc.br = br
	lc.agent = agent
	lc.mu.Unlock()
	lc.sink.setTrack(br, newAudioCodec(br.audioMime))
	if old != nil {
		old.Close()
	}
	br.RequestKeyframe()
	return answer, nil
}

func agentName(a string) string {
	if a == "" {
		return "atendente"
	}
	return a
}

// ---------------------------------------------------------------- ações das rotas

// Media é o pedido do navegador: a oferta SDP e quem é o atendente.
type Media struct {
	SDP   string // oferta SDP do navegador (com os candidatos)
	Agent string // identificação do atendente (id ou nome), vai nos eventos e no usuário TURN
}

// Result é a resposta das ações que ligam o navegador.
type Result struct {
	Call map[string]any `json:"call"`
	SDP  string         `json:"sdp"`
	Type string         `json:"type"`
	Ice  []ICEServer    `json:"iceServers,omitempty"`
}

func (m *Manager) get(inst, callID string) (*liveCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if lc := m.live[key(inst, callID)]; lc != nil {
		return lc, nil
	}
	return nil, ErrNotLive
}

func (m *Manager) client(inst string) (*meowcaller.Client, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.clients[inst]; c != nil {
		return c, nil
	}
	return nil, ErrDisabled
}

func (m *Manager) result(lc *liveCall, sdp string) (*Result, error) {
	rec, err := m.Repo.CallByID(lc.inst, lc.id)
	if err != nil {
		return nil, err
	}
	return &Result{Call: callJSON(rec, lc), SDP: sdp, Type: "answer"}, nil
}

// Answer atende a chamada no navegador do atendente. Só o primeiro que atender leva a chamada.
func (m *Manager) Answer(inst, callID string, md Media) (*Result, error) {
	lc, err := m.get(inst, callID)
	if err != nil {
		return nil, err
	}
	lc.mu.Lock()
	taken := lc.claimed || lc.fromMe
	lc.claimed = true // só o primeiro atendente passa daqui
	lc.mu.Unlock()
	if taken {
		return nil, ErrTaken
	}
	release := func() {
		lc.mu.Lock()
		lc.claimed = false
		lc.mu.Unlock()
	}
	answer, err := lc.attach(md.Agent, md.SDP)
	if err != nil {
		release()
		return nil, err
	}
	if err := lc.call.Answer(); err != nil {
		_ = lc.call.Hangup()
		return nil, fmt.Errorf("o WhatsApp não aceitou atender: %w", err)
	}
	lc.onAnswered()
	return m.result(lc, answer)
}

// Start liga para um número. O áudio do navegador começa a passar quando o contato atender.
func (m *Manager) Start(ctx context.Context, sess *engine.Session, number string, video bool, md Media) (*Result, error) {
	mc, err := m.client(sess.ID)
	if err != nil {
		return nil, err
	}
	if _, err := offerAudioMime(md.SDP); err != nil {
		return nil, err
	}
	jid, err := sess.ResolveJID(ctx, number)
	if err != nil {
		return nil, err
	}
	cli, err := sess.Client()
	if err != nil {
		return nil, err
	}
	c, err := mc.CallWithOptions(ctx, jid.ToNonAD().String(), meowcaller.CallOptions{Video: video})
	if err != nil {
		return nil, fmt.Errorf("o WhatsApp não completou a ligação: %w", err)
	}
	lc := m.track(sess, cli, c, true, video, jid.ToNonAD().String())
	if _, err := m.Repo.CreateCall(&store.Call{InstanceID: lc.inst, CallID: lc.id, RemoteJid: lc.remote, FromMe: true, IsVideo: video}); err != nil {
		sess.Logf("ERROR", "não consegui registrar a chamada %s: %v", lc.id, err)
	}
	answer, err := lc.attach(md.Agent, md.SDP)
	if err != nil {
		_ = c.Hangup()
		return nil, err
	}
	sess.Logf("INFO", "ligando para %s", lc.remote)
	if rec, err := m.Repo.CallByID(lc.inst, lc.id); err == nil {
		sess.Emit(EventRinging, callJSON(rec, lc))
	}
	return m.result(lc, answer)
}

// Transfer passa a chamada para o navegador de outro atendente sem derrubar o contato.
func (m *Manager) Transfer(inst, callID string, md Media) (*Result, error) {
	lc, err := m.get(inst, callID)
	if err != nil {
		return nil, err
	}
	lc.mu.Lock()
	ready := lc.br != nil
	lc.mu.Unlock()
	if !ready {
		return nil, ErrNoMedia
	}
	answer, err := lc.attach(md.Agent, md.SDP)
	if err != nil {
		return nil, err
	}
	lc.sess.Logf("INFO", "chamada %s transferida para %s", lc.id, agentName(md.Agent))
	lc.emitMedia(map[string]any{"transferred": true})
	return m.result(lc, answer)
}

// Reject recusa uma chamada que ainda está tocando.
func (m *Manager) Reject(inst, callID string) error {
	lc, err := m.get(inst, callID)
	if err != nil {
		return err
	}
	return lc.call.Reject()
}

// Hangup encerra a chamada (atendida ou ainda chamando).
func (m *Manager) Hangup(inst, callID string) error {
	lc, err := m.get(inst, callID)
	if err != nil {
		return err
	}
	return lc.call.Hangup()
}

// Mute liga ou desliga o microfone do atendente (o contato passa a ouvir silêncio).
func (m *Manager) Mute(inst, callID string, muted bool) error {
	lc, err := m.get(inst, callID)
	if err != nil {
		return err
	}
	lc.src.setMuted(muted)
	lc.mu.Lock()
	lc.muted = muted
	lc.mu.Unlock()
	lc.emitMedia(nil)
	return nil
}

// Camera liga ou desliga a câmera do atendente. Numa chamada de voz, ligar pede ao contato
// para passar a vídeo; se o contato pediu primeiro, ligar aceita o pedido.
func (m *Manager) Camera(inst, callID string, on bool) error {
	lc, err := m.get(inst, callID)
	if err != nil {
		return err
	}
	lc.mu.Lock()
	upgrade := lc.upgrade
	lc.mu.Unlock()
	switch {
	case !on:
		err = lc.call.SetVideoEnabled(false)
	case upgrade:
		err = lc.call.AcceptVideo()
	case lc.call.IsSendingVideo():
		err = lc.call.SetVideoEnabled(true)
	default:
		err = lc.call.StartVideo()
	}
	if err != nil {
		return err
	}
	lc.mu.Lock()
	lc.camera = on
	if on {
		lc.upgrade = false
		lc.video = true
	}
	br := lc.br
	lc.mu.Unlock()
	if on && br != nil {
		br.RequestKeyframe()
	}
	lc.emitMedia(nil)
	return nil
}

// AddICE recebe um candidato extra do navegador.
func (m *Manager) AddICE(inst, callID string, c webrtc.ICECandidateInit) error {
	lc, err := m.get(inst, callID)
	if err != nil {
		return err
	}
	br := lc.bridgeRef()
	if br == nil {
		return ErrNoMedia
	}
	return br.AddCandidate(c)
}

// Live devolve as chamadas em andamento da instância.
func (m *Manager) Live(inst string) []map[string]any {
	m.mu.Lock()
	list := make([]*liveCall, 0)
	for _, lc := range m.live {
		if lc.inst == inst {
			list = append(list, lc)
		}
	}
	m.mu.Unlock()
	out := make([]map[string]any, 0, len(list))
	for _, lc := range list {
		if rec, err := m.Repo.CallByID(lc.inst, lc.id); err == nil {
			out = append(out, callJSON(rec, lc))
		}
	}
	return out
}

// State devolve os dados ao vivo de uma chamada (nil se não estiver em andamento).
func (m *Manager) State(inst, callID string) map[string]any {
	lc, err := m.get(inst, callID)
	if err != nil {
		return nil
	}
	return lc.state()
}

func (lc *liveCall) state() map[string]any {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	return map[string]any{
		"agent": lc.agent, "muted": lc.muted, "camera": lc.camera, "videoRequest": lc.upgrade,
		"browser": lc.br != nil, "peerVideo": lc.call.IsReceivingVideo(),
	}
}

// CallJSON é o formato de uma chamada nas rotas e nos eventos.
func CallJSON(c *store.Call) map[string]any { return callJSON(c, nil) }

func callJSON(c *store.Call, lc *liveCall) map[string]any {
	out := map[string]any{
		"id": c.CallID, "callId": c.CallID, "remoteJid": c.RemoteJid, "number": strings.SplitN(c.RemoteJid, "@", 2)[0],
		"fromMe": c.FromMe, "isVideo": c.IsVideo, "status": c.Status, "reason": c.Reason,
		"startedAt": c.StartedAt.Unix(), "durationSec": c.DurationSec, "live": lc != nil && !c.Final(),
	}
	if c.AnsweredAt != nil {
		out["answeredAt"] = c.AnsweredAt.Unix()
	}
	if c.EndedAt != nil {
		out["endedAt"] = c.EndedAt.Unix()
	}
	if lc != nil {
		for k, v := range lc.state() {
			out[k] = v
		}
		if lc.video {
			out["isVideo"] = true
		}
	}
	return out
}

// Ring faz o número tocar por alguns segundos e desliga, sem atendente (rota v2
// /call/offer: avisos e testes). Se o contato atender antes, a ligação é encerrada na hora.
func (m *Manager) Ring(ctx context.Context, sess *engine.Session, number string, video bool, dur time.Duration) (map[string]any, error) {
	mc, err := m.client(sess.ID)
	if err != nil {
		return nil, err
	}
	jid, err := sess.ResolveJID(ctx, number)
	if err != nil {
		return nil, err
	}
	cli, err := sess.Client()
	if err != nil {
		return nil, err
	}
	if dur <= 0 || dur > 60*time.Second {
		dur = 15 * time.Second
	}
	c, err := mc.CallWithOptions(ctx, jid.ToNonAD().String(), meowcaller.CallOptions{Video: video})
	if err != nil {
		return nil, fmt.Errorf("o WhatsApp não completou a ligação: %w", err)
	}
	lc := m.track(sess, cli, c, true, video, jid.ToNonAD().String())
	rec, err := m.Repo.CreateCall(&store.Call{InstanceID: lc.inst, CallID: lc.id, RemoteJid: lc.remote, FromMe: true, IsVideo: video})
	if err != nil {
		_ = c.Hangup()
		return nil, err
	}
	c.OnPeerAccept(func() {
		lc.onAnswered()
		_ = c.Hangup()
	})
	time.AfterFunc(dur, func() { _ = c.Hangup() })
	sess.Emit(EventRinging, callJSON(rec, lc))
	return callJSON(rec, lc), nil
}
