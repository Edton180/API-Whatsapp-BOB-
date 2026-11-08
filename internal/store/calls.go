// API WhatsApp BOB - Desenvolvido por Edton Braz.

package store

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Status de uma chamada.
const (
	CallRinging   = "ringing"
	CallConnected = "connected"
	CallEnded     = "ended"
	CallRejected  = "rejected"
	CallMissed    = "missed"

	// ReasonElsewhere é o motivo de quando a chamada foi atendida em outro aparelho (celular).
	ReasonElsewhere = "accepted_elsewhere"
)

// Call é uma chamada de voz ou vídeo da instância. Só os dados da chamada ficam guardados:
// áudio e vídeo passam em tempo real e nunca são gravados.
type Call struct {
	ID          uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	InstanceID  string     `json:"instanceId" gorm:"size:64;not null;uniqueIndex:idx_bob_call,priority:1;index:idx_bob_call_started,priority:1"`
	CallID      string     `json:"callId" gorm:"size:128;not null;uniqueIndex:idx_bob_call,priority:2"`
	RemoteJid   string     `json:"remoteJid" gorm:"size:128"`
	FromMe      bool       `json:"fromMe"`
	IsVideo     bool       `json:"isVideo"`
	Status      string     `json:"status" gorm:"size:16;index"`
	Reason      string     `json:"reason,omitempty" gorm:"size:64"`
	StartedAt   time.Time  `json:"startedAt" gorm:"index:idx_bob_call_started,priority:2"`
	AnsweredAt  *time.Time `json:"answeredAt"`
	EndedAt     *time.Time `json:"endedAt"`
	DurationSec int        `json:"durationSec"`
	CreatedAt   time.Time  `json:"createdAt" gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `json:"updatedAt" gorm:"autoUpdateTime"`
}

func (Call) TableName() string { return "bob_calls" }

// Final diz se o status encerra a chamada (não muda mais).
func (c *Call) Final() bool {
	return c.Status == CallEnded || c.Status == CallRejected || c.Status == CallMissed
}

// ErrCallNotFound é devolvido quando a chamada não existe na instância.
var ErrCallNotFound = errors.New("chamada não encontrada")

// CreateCall grava uma chamada nova. Se o mesmo callId já existe (evento repetido), devolve a
// que já estava gravada sem duplicar.
func (r *Repo) CreateCall(c *Call) (*Call, error) {
	if c.StartedAt.IsZero() {
		c.StartedAt = time.Now()
	}
	if c.Status == "" {
		c.Status = CallRinging
	}
	if err := r.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(c).Error; err != nil {
		return nil, err
	}
	return r.CallByID(c.InstanceID, c.CallID)
}

// CallByID busca uma chamada pelo callId do WhatsApp.
func (r *Repo) CallByID(instanceID, callID string) (*Call, error) {
	var c Call
	if err := r.DB.Where("instance_id = ? AND call_id = ?", instanceID, callID).First(&c).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCallNotFound
		}
		return nil, err
	}
	return &c, nil
}

// MarkCallAnswered passa a chamada para "connected", só se ela ainda estiver tocando.
func (r *Repo) MarkCallAnswered(instanceID, callID string, at time.Time) (*Call, error) {
	err := r.DB.Model(&Call{}).
		Where("instance_id = ? AND call_id = ? AND status = ?", instanceID, callID, CallRinging).
		Updates(map[string]any{"status": CallConnected, "answered_at": at}).Error
	if err != nil {
		return nil, err
	}
	return r.CallByID(instanceID, callID)
}

// FinishCall encerra a chamada com o status final e calcula a duração. Uma chamada já
// encerrada não muda (eventos repetidos ou fora de ordem).
func (r *Repo) FinishCall(instanceID, callID, status, reason string, at time.Time) (*Call, error) {
	err := r.DB.Transaction(func(tx *gorm.DB) error {
		var c Call
		if err := tx.Where("instance_id = ? AND call_id = ?", instanceID, callID).First(&c).Error; err != nil {
			return err
		}
		if c.Final() {
			return nil
		}
		if status == CallEnded && c.AnsweredAt == nil && reason != ReasonElsewhere {
			if c.FromMe {
				status = CallEnded // ligação feita e não atendida: encerrada sem duração
			} else {
				status = CallMissed
			}
		}
		fields := map[string]any{"status": status, "ended_at": at, "reason": reason}
		if c.AnsweredAt != nil {
			fields["duration_sec"] = int(at.Sub(*c.AnsweredAt).Seconds())
		}
		return tx.Model(&Call{}).Where("id = ?", c.ID).Updates(fields).Error
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCallNotFound
		}
		return nil, err
	}
	return r.CallByID(instanceID, callID)
}

// CallQuery filtra o histórico de chamadas.
type CallQuery struct {
	Status string
	Remote string
	Video  *bool
	Since  time.Time
	Limit  int
	Offset int
}

// Calls lista as chamadas, as mais novas primeiro, com o total que casa com o filtro.
func (r *Repo) Calls(instanceID string, q CallQuery) ([]Call, int64, error) {
	tx := r.DB.Model(&Call{}).Where("instance_id = ?", instanceID)
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.Remote != "" {
		tx = tx.Where("remote_jid = ?", q.Remote)
	}
	if q.Video != nil {
		tx = tx.Where("is_video = ?", *q.Video)
	}
	if !q.Since.IsZero() {
		tx = tx.Where("started_at >= ?", q.Since)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var list []Call
	err := tx.Order("started_at DESC, id DESC").Limit(limit).Offset(max(q.Offset, 0)).Find(&list).Error
	return list, total, err
}

// CloseOpenCalls encerra as chamadas que ficaram abertas (a BOB reiniciou no meio delas).
func (r *Repo) CloseOpenCalls(at time.Time) ([]Call, error) {
	var open []Call
	if err := r.DB.Where("status IN ?", []string{CallRinging, CallConnected}).Find(&open).Error; err != nil {
		return nil, err
	}
	out := make([]Call, 0, len(open))
	for _, c := range open {
		if fc, err := r.FinishCall(c.InstanceID, c.CallID, CallEnded, "server_restart", at); err == nil {
			out = append(out, *fc)
		}
	}
	return out, nil
}
