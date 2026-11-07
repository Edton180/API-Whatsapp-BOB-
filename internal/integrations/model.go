// Package integrations: integrações nativas da API WhatsApp BOB (Chatwoot, Typebot e N8N).
//
// Integrações nativas da BOB com Chatwoot, Typebot e N8N. As rotas (/chatwoot/set, /typebot/set,
// /n8n/set) usam os mesmos nomes conhecidos do mercado, o que facilita trocar de API sem refazer fluxos.
package integrations

import (
	"encoding/json"
	"errors"
	"sync"
	"time"

	"gorm.io/gorm"
)

const (
	KindChatwoot = "chatwoot"
	KindTypebot  = "typebot"
	KindN8N      = "n8n"
)

// Integration guarda a configuração de uma integração de uma instância.
type Integration struct {
	InstanceId string    `json:"instanceId" gorm:"primaryKey;size:64"`
	Kind       string    `json:"kind" gorm:"primaryKey;size:16"`
	Enabled    bool      `json:"enabled"`
	Config     string    `json:"-" gorm:"type:text"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (Integration) TableName() string { return "bob_integrations" }

// Session é a conversa em andamento de um contato com um bot (Typebot ou N8N).
type Session struct {
	InstanceId string `gorm:"primaryKey;size:64"`
	Kind       string `gorm:"primaryKey;size:16"`
	RemoteJid  string `gorm:"primaryKey;size:128"`
	SessionId  string `gorm:"size:255"`
	UpdatedAt  time.Time
}

func (Session) TableName() string { return "bob_bot_sessions" }

// BotTrigger decide quando o bot responde.
type BotTrigger struct {
	TriggerType     string `json:"triggerType"`     // all (padrão), keyword, none
	TriggerOperator string `json:"triggerOperator"` // equals, contains, startsWith (padrão contains)
	TriggerValue    string `json:"triggerValue"`
	KeywordFinish   string `json:"keywordFinish"` // encerra a sessão, ex.: #sair
	ExpireMinutes   int    `json:"expire"`        // sessão expira sem resposta (0 = nunca)
	IgnoreGroups    *bool  `json:"ignoreGroups"`  // padrão true
	ListeningFromMe bool   `json:"listeningFromMe"`
	DelayMessage    int    `json:"delayMessage"` // ms entre mensagens
}

type ChatwootConfig struct {
	Url           string `json:"url"`
	AccountId     string `json:"accountId"`
	Token         string `json:"token"`
	InboxId       int    `json:"inboxId"`
	SignMsg       bool   `json:"signMsg"`
	ImportFromMe  *bool  `json:"importFromMe"` // mensagens enviadas pelo celular (padrão true)
	IgnoreGroups  *bool  `json:"ignoreGroups"`
	WebhookSecret string `json:"webhookSecret,omitempty"`
}

type TypebotConfig struct {
	BotTrigger
	Url      string `json:"url"`
	Typebot  string `json:"typebot"` // id público do fluxo
	ApiToken string `json:"apiToken,omitempty"`
}

type N8NConfig struct {
	BotTrigger
	WebhookUrl string `json:"webhookUrl"`
	BasicUser  string `json:"basicAuthUser,omitempty"`
	BasicPass  string `json:"basicAuthPass,omitempty"`
}

func boolOr(v *bool, def bool) bool {
	if v == nil {
		return def
	}
	return *v
}

func decode[T any](it *Integration) (T, error) {
	var c T
	if it == nil {
		return c, errors.New("integração não configurada")
	}
	err := json.Unmarshal([]byte(it.Config), &c)
	return c, err
}

var ErrNotFound = errors.New("não encontrado")

// Store guarda integrações e sessões.
type Store interface {
	Get(instanceID, kind string) (*Integration, error)
	Save(it *Integration) error
	Delete(instanceID, kind string) error
	GetSession(instanceID, kind, jid string) (*Session, error)
	SaveSession(s *Session) error
	DeleteSession(instanceID, kind, jid string) error
}

// GormStore usa o mesmo banco da BOB (tabelas bob_integrations e bob_bot_sessions).
type GormStore struct{ DB *gorm.DB }

func NewGormStore(db *gorm.DB) (*GormStore, error) {
	if err := db.AutoMigrate(&Integration{}, &Session{}); err != nil {
		return nil, err
	}
	return &GormStore{DB: db}, nil
}

func (s *GormStore) Get(id, kind string) (*Integration, error) {
	var it Integration
	err := s.DB.Where("instance_id = ? AND kind = ?", id, kind).First(&it).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &it, err
}

func (s *GormStore) Save(it *Integration) error { return s.DB.Save(it).Error }

func (s *GormStore) Delete(id, kind string) error {
	s.DB.Where("instance_id = ? AND kind = ?", id, kind).Delete(&Session{})
	return s.DB.Where("instance_id = ? AND kind = ?", id, kind).Delete(&Integration{}).Error
}

func (s *GormStore) GetSession(id, kind, jid string) (*Session, error) {
	var ss Session
	err := s.DB.Where("instance_id = ? AND kind = ? AND remote_jid = ?", id, kind, jid).First(&ss).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &ss, err
}

func (s *GormStore) SaveSession(ss *Session) error { return s.DB.Save(ss).Error }

func (s *GormStore) DeleteSession(id, kind, jid string) error {
	return s.DB.Where("instance_id = ? AND kind = ? AND remote_jid = ?", id, kind, jid).Delete(&Session{}).Error
}

// MemoryStore é usado nos testes e quando não há banco.
type MemoryStore struct {
	mu       sync.Mutex
	items    map[string]Integration
	sessions map[string]Session
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{items: map[string]Integration{}, sessions: map[string]Session{}}
}

func (m *MemoryStore) Get(id, kind string) (*Integration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	it, ok := m.items[id+"|"+kind]
	if !ok {
		return nil, ErrNotFound
	}
	return &it, nil
}

func (m *MemoryStore) Save(it *Integration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[it.InstanceId+"|"+it.Kind] = *it
	return nil
}

func (m *MemoryStore) Delete(id, kind string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, id+"|"+kind)
	return nil
}

func (m *MemoryStore) GetSession(id, kind, jid string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id+"|"+kind+"|"+jid]
	if !ok {
		return nil, ErrNotFound
	}
	return &s, nil
}

func (m *MemoryStore) SaveSession(s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.InstanceId+"|"+s.Kind+"|"+s.RemoteJid] = *s
	return nil
}

func (m *MemoryStore) DeleteSession(id, kind, jid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id+"|"+kind+"|"+jid)
	return nil
}
