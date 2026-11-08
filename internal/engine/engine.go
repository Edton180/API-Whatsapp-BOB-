// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package engine é o motor da BOB: cria e mantém as sessões do WhatsApp sobre a biblioteca
// whatsmeow (go.mau.fi/whatsmeow, MPL 2.0) e transforma os eventos em webhooks.
package engine

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/edton180/api-whatsapp-bob/internal/config"
	"github.com/edton180/api-whatsapp-bob/internal/events"
	bobstore "github.com/edton180/api-whatsapp-bob/internal/store"
)

var (
	ErrNotLogged    = errors.New("instância não conectada ao WhatsApp: leia o QR Code ou use o código de pareamento")
	ErrNotConnected = errors.New("instância sem conexão com o WhatsApp no momento, tente de novo em instantes")
	ErrLogged       = errors.New("instância já está conectada a um número")
)

// Engine guarda todas as sessões ativas.
type Engine struct {
	Cfg       *config.Config
	Repo      *bobstore.Repo
	Events    *events.Dispatcher
	container *sqlstore.Container
	waLog     waLog.Logger
	Started   time.Time

	mu       sync.RWMutex
	sessions map[string]*Session
}

// New abre o banco das sessões do WhatsApp e prepara o motor.
func New(cfg *config.Config, repo *bobstore.Repo, disp *events.Dispatcher) (*Engine, error) {
	var db *sql.DB
	var dialect string
	var err error
	if cfg.AuthDB != "" {
		dialect = "postgres"
		db, err = sql.Open("postgres", cfg.AuthDB)
	} else {
		if err = os.MkdirAll(cfg.DataDir, 0o755); err != nil {
			return nil, err
		}
		dialect = "sqlite3"
		db, err = sql.Open("sqlite3", "file:"+filepath.Join(cfg.DataDir, "main.db")+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL")
		if err == nil {
			db.SetMaxOpenConns(1)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("banco das sessões: %w", err)
	}
	level := cfg.LogLevel
	if level == "" {
		level = "WARN"
	}
	wl := waLog.Stdout("WhatsApp", level, true)
	container := sqlstore.NewWithDB(db, dialect, wl.Sub("Banco"))
	if err := container.Upgrade(context.Background()); err != nil {
		return nil, fmt.Errorf("tabelas das sessões: %w", err)
	}
	configureDevice(cfg)
	return &Engine{
		Cfg: cfg, Repo: repo, Events: disp, container: container, waLog: wl,
		Started: time.Now(), sessions: map[string]*Session{},
	}, nil
}

// configureDevice define como o aparelho aparece em "Aparelhos conectados".
func configureDevice(cfg *config.Config) {
	name := cfg.OSName
	if name == "" {
		name = "API WhatsApp BOB"
	}
	store.DeviceProps.Os = proto.String(name)
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
}

// refreshVersion usa a versão atual do WhatsApp Web (versão antiga faz o QR Code ser recusado).
func refreshVersion() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if v, err := whatsmeow.GetLatestVersion(ctx, &http.Client{Timeout: 15 * time.Second}); err == nil && v != nil {
		store.SetWAVersion(*v)
	} else if err != nil {
		log.Printf("[motor] não consegui ler a versão atual do WhatsApp Web, usando a da biblioteca: %v", err)
	}
}

// Start lê a versão atual do WhatsApp Web e reconecta as instâncias que já tinham sessão.
func (e *Engine) Start() {
	refreshVersion()
	versionMu.Lock()
	versionAt = time.Now()
	versionMu.Unlock()
	go e.purgeLoop()
	go e.maintain()
	if !e.Cfg.ConnectOnStartup {
		return
	}
	list, err := e.Repo.All()
	if err != nil {
		log.Printf("[motor] não consegui listar as instâncias: %v", err)
		return
	}
	for _, inst := range list {
		if inst.Jid == "" {
			continue
		}
		go func(inst *bobstore.Instance) {
			if _, err := e.Connect(inst, ConnectOptions{}); err != nil {
				log.Printf("[motor] %s: não reconectou: %v", inst.Name, err)
			}
		}(inst)
		time.Sleep(300 * time.Millisecond)
	}
}

// Stop desliga todas as sessões sem encerrar o login no celular.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, s := range e.sessions {
		if c := s.client(); c != nil {
			c.Disconnect()
		}
	}
}

// Session devolve a sessão da instância (cria uma vazia se ainda não houver).
func (e *Engine) Session(inst *bobstore.Instance) *Session {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.sessions[inst.Id]
	if !ok {
		s = newSession(e, inst)
		e.sessions[inst.Id] = s
	} else {
		s.setInstance(inst)
	}
	return s
}

// Lookup devolve a sessão existente, sem criar.
func (e *Engine) Lookup(id string) *Session {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.sessions[id]
}

func (e *Engine) drop(id string) {
	e.mu.Lock()
	delete(e.sessions, id)
	e.mu.Unlock()
}

// Stats resume o servidor para o painel.
func (e *Engine) Stats() (total, connected int) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, s := range e.sessions {
		if c := s.client(); c != nil && c.IsConnected() && c.IsLoggedIn() {
			connected++
		}
	}
	list, _ := e.Repo.All()
	return len(list), connected
}

// ConnectOptions muda o webhook e os eventos ao conectar (nil mantém o que já existe).
type ConnectOptions struct {
	Webhook   *string
	Subscribe []string
	Websocket *bool
}

// Connect inicia (ou retoma) a sessão da instância.
func (e *Engine) Connect(inst *bobstore.Instance, opt ConnectOptions) (*Session, error) {
	fields := map[string]any{}
	if opt.Webhook != nil {
		inst.Webhook = strings.TrimSpace(*opt.Webhook)
		fields["webhook"] = inst.Webhook
	}
	if opt.Subscribe != nil {
		inst.Events = events.NormalizeSubscribe(opt.Subscribe)
		fields["events"] = inst.Events
	}
	if opt.Websocket != nil {
		inst.WebSocketEnable = map[bool]string{true: "enabled", false: ""}[*opt.Websocket]
		fields["web_socket_enable"] = inst.WebSocketEnable
	}
	if len(fields) > 0 {
		if err := e.Repo.Update(inst.Id, fields); err != nil {
			return nil, err
		}
	}
	s := e.Session(inst)
	return s, s.start()
}

// Delete encerra a sessão, apaga o aparelho e a instância.
func (e *Engine) Delete(ctx context.Context, inst *bobstore.Instance) error {
	if s := e.Lookup(inst.Id); s != nil {
		if c := s.client(); c != nil {
			if c.IsLoggedIn() {
				_ = c.Logout(ctx)
			}
			c.Disconnect()
			if c.Store != nil && c.Store.ID != nil {
				_ = e.container.DeleteDevice(ctx, c.Store)
			}
		}
		s.stopQR()
		e.drop(inst.Id)
	} else if inst.Jid != "" {
		if jid, err := types.ParseJID(inst.Jid); err == nil {
			if dev, err := e.container.GetDevice(ctx, jid); err == nil && dev != nil {
				_ = e.container.DeleteDevice(ctx, dev)
			}
		}
	}
	return e.Repo.Delete(inst.Id)
}

// ---------------------------------------------------------------- sessão

// Session é um número conectado (ou conectando) pela whatsmeow.
type Session struct {
	e  *Engine
	ID string

	mu       sync.Mutex
	inst     bobstore.Instance
	cli      *whatsmeow.Client
	qrImage  string
	qrCode   string
	qrCount  int
	qrCancel context.CancelFunc

	logs      *ring
	recent    *messageCache
	lastEvent int64    // unix do último evento do WhatsApp (números "zumbis")
	jids      sync.Map // número -> JID confirmado (cache do /user/check)
}

func newSession(e *Engine, inst *bobstore.Instance) *Session {
	return &Session{e: e, ID: inst.Id, inst: *inst, logs: newRing(300), recent: newMessageCache(3000)}
}

func (s *Session) setInstance(inst *bobstore.Instance) {
	s.mu.Lock()
	s.inst = *inst
	s.mu.Unlock()
}

// Instance devolve uma cópia dos dados da instância.
func (s *Session) Instance() bobstore.Instance {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inst
}

func (s *Session) client() *whatsmeow.Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cli
}

// Client devolve o cliente pronto para uso (conectado e logado).
func (s *Session) Client() (*whatsmeow.Client, error) {
	c := s.client()
	if c == nil || !c.IsLoggedIn() {
		if c != nil && c.Store != nil && c.Store.ID != nil && !c.IsConnected() {
			return nil, ErrNotConnected
		}
		return nil, ErrNotLogged
	}
	if !c.IsConnected() {
		return nil, ErrNotConnected
	}
	return c, nil
}

// start cria o cliente (se preciso) e abre a conexão. Sem sessão salva, gera QR Codes.
func (s *Session) start() error {
	s.mu.Lock()
	cli := s.cli
	inst := s.inst
	s.mu.Unlock()

	if cli != nil && cli.IsConnected() {
		return nil
	}
	ctx := context.Background()
	if cli == nil {
		var dev *store.Device
		if inst.Jid != "" {
			if jid, err := types.ParseJID(inst.Jid); err == nil {
				dev, _ = s.e.container.GetDevice(ctx, jid)
			}
		}
		if dev == nil {
			dev = s.e.container.NewDevice()
		}
		cli = whatsmeow.NewClient(dev, s.e.waLog.Sub(inst.Name))
		cli.EnableAutoReconnect = true
		cli.AutoTrustIdentity = true
		cli.AddEventHandler(s.handle)
		if err := applyProxy(cli, proxyFor(s.e.Cfg, inst.Proxy)); err != nil {
			s.logf("WARN", "proxy ignorado: %v", err)
		}
		s.mu.Lock()
		s.cli = cli
		s.mu.Unlock()
	}

	if cli.Store.ID == nil {
		s.stopQR()
		qctx, cancel := context.WithCancel(context.Background())
		ch, err := cli.GetQRChannel(qctx)
		if err != nil {
			cancel()
			if !errors.Is(err, whatsmeow.ErrQRStoreContainsID) {
				return err
			}
		} else {
			s.mu.Lock()
			s.qrCancel, s.qrCount, s.qrImage, s.qrCode = cancel, 0, "", ""
			s.mu.Unlock()
			go s.watchQR(ch)
		}
	}
	if err := cli.Connect(); err != nil {
		// Cliente que não conectou é descartado: o próximo /instance/connect cria outro do zero.
		s.stopQR()
		s.mu.Lock()
		if s.cli == cli {
			s.cli = nil
		}
		s.mu.Unlock()
		s.logf("ERROR", "falha ao conectar: %v", err)
		return fmt.Errorf("não consegui conectar ao WhatsApp: %w", err)
	}
	s.logf("INFO", "conectando ao WhatsApp")
	return nil
}

func (s *Session) stopQR() {
	s.mu.Lock()
	if s.qrCancel != nil {
		s.qrCancel()
		s.qrCancel = nil
	}
	s.mu.Unlock()
}

func (s *Session) watchQR(ch <-chan whatsmeow.QRChannelItem) {
	max := s.e.Cfg.QRMaxCount
	for item := range ch {
		switch item.Event {
		case "code":
			s.mu.Lock()
			s.qrCount++
			count := s.qrCount
			s.mu.Unlock()
			if max > 0 && count > max {
				s.clearQR()
				s.emit("QRTimeout", map[string]any{"reason": "limite de QR Codes atingido", "count": count - 1})
				s.logf("WARN", "QR Code não lido após %d tentativas; conexão encerrada", max)
				if c := s.client(); c != nil {
					c.Disconnect()
				}
				s.stopQR()
				return
			}
			png, err := qrcode.Encode(item.Code, qrcode.Medium, 320)
			if err != nil {
				continue
			}
			img := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
			s.mu.Lock()
			s.qrImage, s.qrCode = img, item.Code
			s.mu.Unlock()
			_ = s.e.Repo.Update(s.ID, map[string]any{"qrcode": img})
			s.emit("QRCode", map[string]any{"qrcode": img, "code": item.Code, "count": count, "maxCount": max, "timeout": int(item.Timeout.Seconds())})
		case "success":
			s.clearQR()
			s.emit("QRSuccess", map[string]any{})
		case "timeout":
			s.clearQR()
			s.emit("QRTimeout", map[string]any{"reason": "tempo do QR Code esgotado"})
		default:
			if item.Event == "err-client-outdated" {
				// o evento ClientOutdated cuida de renovar a versão e reconectar
				s.logf("ERROR", "QR Code recusado: versão do cliente desatualizada")
			}
			if strings.HasPrefix(item.Event, "err") {
				msg := item.Event
				if item.Error != nil {
					msg = item.Error.Error()
				}
				s.clearQR()
				s.emit("PairError", map[string]any{"error": msg})
				s.logf("ERROR", "pareamento recusado: %s", msg)
			}
		}
	}
}

func (s *Session) clearQR() {
	s.mu.Lock()
	s.qrImage, s.qrCode = "", ""
	s.mu.Unlock()
	_ = s.e.Repo.Update(s.ID, map[string]any{"qrcode": ""})
}

// QR devolve a imagem (data URI PNG) e o texto do QR Code atual.
func (s *Session) QR() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.qrImage, s.qrCode
}

// Pair gera o código de 8 caracteres para conectar digitando no celular.
func (s *Session) Pair(ctx context.Context, phone string) (string, error) {
	phone = digits(phone)
	if len(phone) < 10 {
		return "", errors.New("informe o número com DDI e DDD, só dígitos (ex.: 5511999998888)")
	}
	if err := s.start(); err != nil {
		return "", err
	}
	cli := s.client()
	if cli == nil {
		return "", ErrNotConnected
	}
	if cli.IsLoggedIn() {
		return "", ErrLogged
	}
	deadline := time.Now().Add(20 * time.Second)
	for !cli.IsConnected() && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
	}
	if !cli.IsConnected() {
		return "", ErrNotConnected
	}
	code, err := cli.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return "", fmt.Errorf("o WhatsApp recusou o pareamento: %w", err)
	}
	s.logf("INFO", "código de pareamento gerado para %s", phone)
	return code, nil
}

// Status resume o estado da conexão.
func (s *Session) Status() map[string]any {
	c := s.client()
	out := map[string]any{"Connected": false, "LoggedIn": false, "Name": "", "Jid": ""}
	if c == nil {
		return out
	}
	out["Connected"] = c.IsConnected()
	out["LoggedIn"] = c.IsLoggedIn()
	if c.Store != nil {
		out["Name"] = c.Store.PushName
		if c.Store.ID != nil {
			out["Jid"] = c.Store.ID.String()
		}
	}
	return out
}

// Disconnect fecha a conexão mantendo o login (reconecta sem QR Code).
func (s *Session) Disconnect() {
	s.stopQR()
	if c := s.client(); c != nil {
		c.Disconnect()
	}
	_ = s.e.Repo.Update(s.ID, map[string]any{"connected": false})
	s.logf("INFO", "desconectada pelo usuário")
}

// Reconnect derruba e abre a conexão de novo.
func (s *Session) Reconnect() error {
	if c := s.client(); c != nil {
		c.Disconnect()
	}
	time.Sleep(500 * time.Millisecond)
	return s.start()
}

// Logout encerra o login no celular (vai precisar de novo QR Code).
func (s *Session) Logout(ctx context.Context) error {
	s.stopQR()
	c := s.client()
	if c != nil {
		if c.IsLoggedIn() {
			if err := c.Logout(ctx); err != nil {
				s.logf("WARN", "logout no servidor do WhatsApp falhou: %v", err)
			}
		}
		c.Disconnect()
	}
	s.mu.Lock()
	s.cli = nil
	s.mu.Unlock()
	_ = s.e.Repo.Update(s.ID, map[string]any{"jid": "", "connected": false, "qrcode": ""})
	s.logf("INFO", "sessão encerrada (logout)")
	return nil
}

// ApplyProxy troca o proxy da instância e reconecta.
func (s *Session) ApplyProxy(raw string) error {
	s.mu.Lock()
	s.inst.Proxy = raw
	c := s.cli
	s.mu.Unlock()
	if c == nil {
		return nil
	}
	if err := applyProxy(c, proxyFor(s.e.Cfg, raw)); err != nil {
		return err
	}
	if c.IsConnected() {
		return s.Reconnect()
	}
	return nil
}

// Logs devolve o histórico recente da instância.
func (s *Session) Logs() []LogEntry { return s.logs.list() }

func (s *Session) logf(level, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.logs.add(LogEntry{Time: time.Now(), Level: level, Message: msg})
	inst := s.Instance()
	log.Printf("[%s] %s: %s", inst.Name, level, msg)
}

// emit envia um evento para o webhook, o WebSocket e as integrações.
func (s *Session) emit(name string, data any) {
	inst := s.Instance()
	if fresh, err := s.e.Repo.ByID(s.ID); err == nil {
		inst = *fresh
		s.setInstance(fresh)
	}
	hook := inst.Webhook
	if hook == "" {
		hook = s.e.Cfg.GlobalWebhook
	}
	s.e.Events.EmitTo(events.Payload{
		Event: name, InstanceID: inst.Id, InstanceName: inst.Name, InstanceToken: inst.Token, Data: data,
	}, events.Target{URL: hook, Subscribed: inst.Events, ByEvents: inst.WebhookByEvents})
}

// ---------------------------------------------------------------- proxy

type proxyConf struct {
	Protocol string `json:"protocol"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func proxyFor(cfg *config.Config, raw string) *proxyConf {
	if strings.TrimSpace(raw) != "" {
		var p proxyConf
		if json.Unmarshal([]byte(raw), &p) == nil && p.Host != "" {
			return &p
		}
	}
	if cfg.Proxy.Host != "" {
		return &proxyConf{cfg.Proxy.Protocol, cfg.Proxy.Host, cfg.Proxy.Port, cfg.Proxy.Username, cfg.Proxy.Password}
	}
	return nil
}

func applyProxy(c *whatsmeow.Client, p *proxyConf) error {
	if p == nil {
		c.SetProxy(nil)
		return nil
	}
	proto := strings.ToLower(p.Protocol)
	if proto == "" {
		proto = "http"
		if p.Port == "1080" || p.Port == "2080" {
			proto = "socks5"
		}
	}
	u := &url.URL{Scheme: proto, Host: p.Host}
	if p.Port != "" {
		u.Host = p.Host + ":" + p.Port
	}
	if p.Username != "" {
		u.User = url.UserPassword(p.Username, p.Password)
	}
	return c.SetProxyAddress(u.String())
}

// ---------------------------------------------------------------- utilidades

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// LogEntry é uma linha do histórico da instância.
type LogEntry struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

type ring struct {
	mu   sync.Mutex
	max  int
	data []LogEntry
}

func newRing(max int) *ring { return &ring{max: max} }

func (r *ring) add(e LogEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, e)
	if len(r.data) > r.max {
		r.data = r.data[len(r.data)-r.max:]
	}
}

func (r *ring) list() []LogEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LogEntry, len(r.data))
	copy(out, r.data)
	return out
}
