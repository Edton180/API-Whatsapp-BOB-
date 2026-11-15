// API WhatsApp BOB - Desenvolvido por Edton Braz.

package store

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Status de uma cobrança.
const (
	ChargePending   = "pending"
	ChargePaid      = "paid"
	ChargeCancelled = "cancelled"
)

// Charge é uma cobrança enviada a um cliente pelo WhatsApp. A BOB monta e entrega a cobrança
// (Pix copia e cola, QR Code, link ou boleto) e guarda a situação; o dinheiro passa direto
// pelo Pix ou pelo gateway de quem cobra, nunca pela BOB.
type Charge struct {
	ID           string     `json:"id" gorm:"primaryKey;size:36"`
	InstanceID   string     `json:"instanceId" gorm:"size:64;not null;index:idx_bob_charge_inst,priority:1;index:idx_bob_charge_ref,priority:1"`
	Number       string     `json:"number" gorm:"size:64;index"`
	RemoteJid    string     `json:"remoteJid" gorm:"size:128"`
	CustomerName string     `json:"customerName" gorm:"size:160"`
	Description  string     `json:"description" gorm:"size:500"`
	AmountCents  int64      `json:"amountCents"`
	Currency     string     `json:"currency" gorm:"size:8"`
	DueDate      string     `json:"dueDate" gorm:"size:10"` // AAAA-MM-DD (vazio = sem vencimento)
	Status       string     `json:"status" gorm:"size:16;index:idx_bob_charge_inst,priority:2"`
	PixCode      string     `json:"pixCode" gorm:"type:text"`
	PaymentURL   string     `json:"paymentUrl" gorm:"type:text"`
	BoletoLine   string     `json:"boletoLine" gorm:"size:120"`
	Reference    string     `json:"reference" gorm:"size:128;index:idx_bob_charge_ref,priority:2"` // id da cobrança no sistema de quem cobra
	Metadata     string     `json:"-" gorm:"type:text"`
	SentCount    int        `json:"sentCount"`
	SentAt       *time.Time `json:"sentAt"`
	PaidAt       *time.Time `json:"paidAt"`
	CancelledAt  *time.Time `json:"cancelledAt"`
	CreatedAt    time.Time  `json:"createdAt" gorm:"autoCreateTime;index:idx_bob_charge_inst,priority:3"`
	UpdatedAt    time.Time  `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (Charge) TableName() string { return "bob_charges" }

func (c *Charge) BeforeCreate(*gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	return nil
}

// Overdue diz se a cobrança está pendente e já venceu.
func (c *Charge) Overdue(now time.Time) bool {
	return c.Status == ChargePending && c.DueDate != "" && c.DueDate < now.Format("2006-01-02")
}

// ChargeSettings guarda o Pix e os textos de cobrança de uma instância.
type ChargeSettings struct {
	InstanceID     string    `json:"-" gorm:"primaryKey;size:64"`
	PixKey         string    `json:"pixKey" gorm:"size:100"`
	PixKeyType     string    `json:"pixKeyType" gorm:"size:16"`
	PixName        string    `json:"pixName" gorm:"size:60"`
	PixCity        string    `json:"pixCity" gorm:"size:40"`
	Message        string    `json:"message" gorm:"type:text"`        // texto da cobrança (vazio = padrão)
	ReceiptMessage string    `json:"receiptMessage" gorm:"type:text"` // texto do recibo (vazio = padrão)
	UpdatedAt      time.Time `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (ChargeSettings) TableName() string { return "bob_charge_settings" }

// ErrChargeNotFound é devolvido quando a cobrança não existe na instância.
var ErrChargeNotFound = errors.New("cobrança não encontrada")

// ChargeSettingsOf devolve a configuração de cobranças da instância (vazia se nunca foi salva).
func (r *Repo) ChargeSettingsOf(instanceID string) ChargeSettings {
	var s ChargeSettings
	r.DB.Where("instance_id = ?", instanceID).Limit(1).Find(&s)
	s.InstanceID = instanceID
	return s
}

func (r *Repo) SaveChargeSettings(s *ChargeSettings) error {
	return r.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(s).Error
}

func (r *Repo) CreateCharge(c *Charge) error { return r.DB.Create(c).Error }

// ChargeByRef acha a cobrança pelo id ou pela referência de quem cobra.
func (r *Repo) ChargeByRef(instanceID, ref string) (*Charge, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, ErrChargeNotFound
	}
	var c Charge
	err := r.DB.Where("instance_id = ? AND (id = ? OR reference = ?)", instanceID, ref, ref).Order("created_at DESC").First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChargeNotFound
		}
		return nil, err
	}
	return &c, nil
}

// ChargeQuery filtra a lista de cobranças.
type ChargeQuery struct {
	Status string // pending, paid, cancelled ou overdue
	Number string
	Limit  int
	Offset int
}

func (r *Repo) Charges(instanceID string, q ChargeQuery) ([]Charge, int64, error) {
	tx := r.DB.Model(&Charge{}).Where("instance_id = ?", instanceID)
	switch q.Status {
	case "":
	case "overdue":
		tx = tx.Where("status = ? AND due_date <> '' AND due_date < ?", ChargePending, time.Now().Format("2006-01-02"))
	default:
		tx = tx.Where("status = ?", q.Status)
	}
	if q.Number != "" {
		tx = tx.Where("number = ?", q.Number)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 50
	}
	var list []Charge
	err := tx.Order("created_at DESC").Limit(q.Limit).Offset(max(q.Offset, 0)).Find(&list).Error
	return list, total, err
}

// SetChargeStatus muda a situação só se a cobrança ainda estiver pendente (evita baixa em dobro).
// Devolve false quando outra chamada já tinha mudado a situação.
func (r *Repo) SetChargeStatus(c *Charge, status string, at time.Time) (bool, error) {
	fields := map[string]any{"status": status, "updated_at": at}
	switch status {
	case ChargePaid:
		fields["paid_at"] = at
	case ChargeCancelled:
		fields["cancelled_at"] = at
	}
	res := r.DB.Model(&Charge{}).Where("id = ? AND status = ?", c.ID, ChargePending).Updates(fields)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 0 {
		return false, nil
	}
	c.Status = status
	if status == ChargePaid {
		c.PaidAt = &at
	} else if status == ChargeCancelled {
		c.CancelledAt = &at
	}
	return true, nil
}

// MarkChargeSent registra mais um envio da cobrança ao cliente.
func (r *Repo) MarkChargeSent(c *Charge, jid string, at time.Time) error {
	c.SentCount++
	c.SentAt = &at
	if jid != "" {
		c.RemoteJid = jid
	}
	return r.DB.Model(&Charge{}).Where("id = ?", c.ID).
		Updates(map[string]any{"sent_count": c.SentCount, "sent_at": at, "remote_jid": c.RemoteJid}).Error
}

// NewID gera um identificador (UUID) para registros da BOB.
func NewID() string { return uuid.NewString() }
