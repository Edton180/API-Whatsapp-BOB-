// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package store guarda as instâncias, etiquetas e integrações da BOB.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Instance é um número de WhatsApp ligado à BOB.
//
// A tabela continua se chamando "instances", com as mesmas colunas das versões anteriores
// da BOB, para que uma atualização mantenha as instâncias e as sessões já conectadas.
type Instance struct {
	Id               string    `json:"id" gorm:"type:uuid;primaryKey"`
	Name             string    `json:"name"`
	Token            string    `json:"token" gorm:"unique"`
	Webhook          string    `json:"webhook"`
	WebSocketEnable  string    `json:"websocketEnable"`
	Jid              string    `json:"jid" gorm:"column:jid"`
	Qrcode           string    `json:"qrcode" gorm:"type:text"`
	Connected        bool      `json:"connected"`
	DisconnectReason string    `json:"disconnect_reason"`
	Events           string    `json:"events"`
	OsName           string    `json:"os_name"`
	Proxy            string    `json:"proxy"`
	ClientName       string    `json:"client_name"`
	CreatedAt        time.Time `json:"createdAt" gorm:"autoCreateTime"`

	AlwaysOnline  bool   `json:"alwaysOnline" gorm:"default:false"`
	RejectCall    bool   `json:"rejectCall" gorm:"default:false"`
	MsgRejectCall string `json:"msgRejectCall" gorm:"default:''"`
	ReadMessages  bool   `json:"readMessages" gorm:"default:false"`
	IgnoreGroups  bool   `json:"ignoreGroups" gorm:"default:false"`
	IgnoreStatus  bool   `json:"ignoreStatus" gorm:"default:false"`
}

func (Instance) TableName() string { return "instances" }

func (m *Instance) BeforeCreate(*gorm.DB) error {
	if m.Id == "" {
		m.Id = uuid.NewString()
	}
	return nil
}

// AdvancedSettings são as opções de comportamento da instância.
type AdvancedSettings struct {
	AlwaysOnline  bool   `json:"alwaysOnline"`
	RejectCall    bool   `json:"rejectCall"`
	MsgRejectCall string `json:"msgRejectCall"`
	ReadMessages  bool   `json:"readMessages"`
	IgnoreGroups  bool   `json:"ignoreGroups"`
	IgnoreStatus  bool   `json:"ignoreStatus"`
}

// Settings devolve as opções avançadas da instância.
func (m *Instance) Settings() AdvancedSettings {
	return AdvancedSettings{m.AlwaysOnline, m.RejectCall, m.MsgRejectCall, m.ReadMessages, m.IgnoreGroups, m.IgnoreStatus}
}

// Apply grava as opções avançadas na instância.
func (m *Instance) Apply(s AdvancedSettings) {
	m.AlwaysOnline, m.RejectCall, m.MsgRejectCall = s.AlwaysOnline, s.RejectCall, s.MsgRejectCall
	m.ReadMessages, m.IgnoreGroups, m.IgnoreStatus = s.ReadMessages, s.IgnoreGroups, s.IgnoreStatus
}

// Label é uma etiqueta do WhatsApp Business (chega pelos eventos de sincronização).
type Label struct {
	InstanceID string `json:"-" gorm:"primaryKey;size:64"`
	LabelID    string `json:"LabelID" gorm:"primaryKey;size:64"`
	Name       string `json:"Name"`
	Color      int32  `json:"Color"`
	Deleted    bool   `json:"Deleted"`
}

func (Label) TableName() string { return "bob_labels" }

// Open abre o banco da BOB: PostgreSQL quando dsn é informado, senão SQLite em dataDir.
func Open(dsn, dataDir string) (*gorm.DB, error) {
	cfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	var db *gorm.DB
	var err error
	if dsn != "" {
		db, err = gorm.Open(postgres.Open(dsn), cfg)
	} else {
		if err = os.MkdirAll(dataDir, 0o755); err != nil {
			return nil, err
		}
		path := filepath.Join(dataDir, "bob.db")
		db, err = gorm.Open(sqlite.Open("file:"+path+"?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL"), cfg)
		if err == nil {
			if sqlDB, e := db.DB(); e == nil {
				sqlDB.SetMaxOpenConns(1)
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("banco da BOB: %w", err)
	}
	if err := db.AutoMigrate(&Instance{}, &Label{}); err != nil {
		return nil, fmt.Errorf("tabelas da BOB: %w", err)
	}
	return db, nil
}

// Repo concentra as consultas de instâncias.
type Repo struct{ DB *gorm.DB }

var ErrNotFound = errors.New("instância não encontrada")

func (r *Repo) All() ([]*Instance, error) {
	var list []*Instance
	err := r.DB.Order("created_at").Find(&list).Error
	return list, err
}

func (r *Repo) ByID(id string) (*Instance, error) {
	var inst Instance
	if err := r.DB.Where("id = ?", id).First(&inst).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &inst, nil
}

// ByIDOrName aceita o id ou o nome da instância.
func (r *Repo) ByIDOrName(v string) (*Instance, error) {
	if _, err := uuid.Parse(v); err == nil {
		if inst, err := r.ByID(v); err == nil {
			return inst, nil
		}
	}
	var inst Instance
	if err := r.DB.Where("name = ?", v).First(&inst).Error; err != nil {
		return nil, ErrNotFound
	}
	return &inst, nil
}

func (r *Repo) ByToken(token string) (*Instance, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	var inst Instance
	if err := r.DB.Where("token = ?", token).First(&inst).Error; err != nil {
		return nil, ErrNotFound
	}
	return &inst, nil
}

func (r *Repo) Create(inst *Instance) error {
	inst.Name = strings.TrimSpace(inst.Name)
	if inst.Name == "" {
		return errors.New("informe o nome da instância")
	}
	var n int64
	r.DB.Model(&Instance{}).Where("name = ?", inst.Name).Count(&n)
	if n > 0 {
		return fmt.Errorf("já existe uma instância com o nome %q", inst.Name)
	}
	if inst.Token == "" {
		inst.Token = RandomToken()
	}
	r.DB.Model(&Instance{}).Where("token = ?", inst.Token).Count(&n)
	if n > 0 {
		return errors.New("este token já está em uso por outra instância")
	}
	return r.DB.Create(inst).Error
}

func (r *Repo) Save(inst *Instance) error { return r.DB.Save(inst).Error }

// Update grava só os campos informados (evita sobrescrever o que outra rotina mudou).
func (r *Repo) Update(id string, fields map[string]any) error {
	return r.DB.Model(&Instance{}).Where("id = ?", id).Updates(fields).Error
}

func (r *Repo) Delete(id string) error {
	r.DB.Where("instance_id = ?", id).Delete(&Label{})
	return r.DB.Where("id = ?", id).Delete(&Instance{}).Error
}

// Labels lista as etiquetas conhecidas da instância.
func (r *Repo) Labels(id string) ([]Label, error) {
	var list []Label
	err := r.DB.Where("instance_id = ? AND deleted = ?", id, false).Order("label_id").Find(&list).Error
	return list, err
}

func (r *Repo) SaveLabel(l *Label) error { return r.DB.Save(l).Error }

// RandomToken gera um token aleatório em hexadecimal maiúsculo.
func RandomToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return strings.ToUpper(hex.EncodeToString(b))
}
