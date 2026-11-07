// API WhatsApp BOB - Desenvolvido por Edton Braz.

package store

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Message é uma mensagem guardada no histórico da instância (recebida, enviada ou vinda da
// sincronização do celular).
type Message struct {
	InstanceID string `json:"instanceId" gorm:"primaryKey;size:64;index:idx_bob_msg_chat,priority:1"`
	ID         string `json:"id" gorm:"primaryKey;size:128"`
	Chat       string `json:"chat" gorm:"size:128;index:idx_bob_msg_chat,priority:2"`
	Sender     string `json:"sender" gorm:"size:128"`
	FromMe     bool   `json:"fromMe"`
	PushName   string `json:"pushName"`
	Type       string `json:"type" gorm:"size:64"`
	Text       string `json:"text" gorm:"type:text"`
	Status     string `json:"status" gorm:"size:24"`
	Timestamp  int64  `json:"timestamp" gorm:"index:idx_bob_msg_chat,priority:3"`
	Edited     bool   `json:"edited"`
	Deleted    bool   `json:"deleted"`
	Raw        []byte `json:"-"` // waE2E.Message em protobuf
}

func (Message) TableName() string { return "bob_messages" }

// Chat é uma conversa conhecida da instância.
type Chat struct {
	InstanceID    string `json:"instanceId" gorm:"primaryKey;size:64;index:idx_bob_chat_last,priority:1"`
	JID           string `json:"jid" gorm:"column:jid;primaryKey;size:128"`
	Name          string `json:"name"`
	LastMessageID string `json:"lastMessageId" gorm:"size:128"`
	LastText      string `json:"lastText" gorm:"type:text"`
	LastFromMe    bool   `json:"lastFromMe"`
	LastSender    string `json:"lastSender" gorm:"size:128"`
	LastTimestamp int64  `json:"lastTimestamp" gorm:"index:idx_bob_chat_last,priority:2"`
	Unread        int    `json:"unreadCount"`
	Archived      bool   `json:"archived"`
	Pinned        bool   `json:"pinned"`
}

func (Chat) TableName() string { return "bob_chats" }

// PollVote é o último voto de cada pessoa numa enquete.
type PollVote struct {
	InstanceID string `json:"-" gorm:"primaryKey;size:64"`
	PollID     string `json:"pollId" gorm:"primaryKey;size:128"`
	Voter      string `json:"voter" gorm:"primaryKey;size:128"`
	PushName   string `json:"pushName"`
	Options    string `json:"-" gorm:"type:text"` // JSON com os nomes escolhidos
	Timestamp  int64  `json:"timestamp"`
}

func (PollVote) TableName() string { return "bob_poll_votes" }

// Selected devolve as opções escolhidas no voto.
func (v PollVote) Selected() []string {
	var out []string
	_ = json.Unmarshal([]byte(v.Options), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

// Ordem dos status de entrega: um status só avança, nunca volta.
var statusRank = map[string]int{"ERROR": 0, "PENDING": 1, "SERVER_ACK": 2, "DELIVERY_ACK": 3, "READ": 4, "PLAYED": 5}

// StatusRank devolve a posição do status na ordem de entrega (-1 se desconhecido).
func StatusRank(s string) int {
	if r, ok := statusRank[s]; ok {
		return r
	}
	return -1
}

// SaveMessage grava ou atualiza uma mensagem, sem rebaixar o status que ela já tinha.
func (r *Repo) SaveMessage(m *Message) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		var old Message
		err := tx.Where("instance_id = ? AND id = ?", m.InstanceID, m.ID).First(&old).Error
		if err == nil {
			if StatusRank(old.Status) > StatusRank(m.Status) {
				m.Status = old.Status
			}
			m.Edited = m.Edited || old.Edited
			m.Deleted = m.Deleted || old.Deleted
			if m.PushName == "" {
				m.PushName = old.PushName
			}
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(m).Error
	})
}

// UpdateStatus avança o status das mensagens informadas.
func (r *Repo) UpdateStatus(instanceID string, ids []string, status string) error {
	if len(ids) == 0 {
		return nil
	}
	lower := []string{}
	for s, rank := range statusRank {
		if rank < StatusRank(status) {
			lower = append(lower, s)
		}
	}
	return r.DB.Model(&Message{}).
		Where("instance_id = ? AND id IN ? AND (status IN ? OR status = '')", instanceID, ids, lower).
		Update("status", status).Error
}

// MarkDeleted marca uma mensagem como apagada para todos.
func (r *Repo) MarkDeleted(instanceID, id string) error {
	return r.DB.Model(&Message{}).Where("instance_id = ? AND id = ?", instanceID, id).
		Updates(map[string]any{"deleted": true}).Error
}

// EditMessage troca o conteúdo de uma mensagem editada.
func (r *Repo) EditMessage(instanceID, id, text string, raw []byte) error {
	fields := map[string]any{"edited": true, "text": text}
	if raw != nil {
		fields["raw"] = raw
	}
	return r.DB.Model(&Message{}).Where("instance_id = ? AND id = ?", instanceID, id).Updates(fields).Error
}

// MessageByID busca uma mensagem pelo id.
func (r *Repo) MessageByID(instanceID, id string) (*Message, error) {
	var m Message
	if err := r.DB.Where("instance_id = ? AND id = ?", instanceID, id).First(&m).Error; err != nil {
		return nil, ErrMessageNotFound
	}
	return &m, nil
}

// ErrMessageNotFound é devolvido quando a mensagem não está no histórico.
var ErrMessageNotFound = errors.New("mensagem não encontrada no histórico")

// MessageQuery filtra o histórico.
type MessageQuery struct {
	Chat   string
	ID     string
	FromMe *bool
	Before int64 // timestamp (segundos), exclusivo
	After  int64 // timestamp (segundos), exclusivo
	Limit  int
	Offset int
}

// Messages lista as mensagens mais novas primeiro e devolve também o total que casa com o filtro.
func (r *Repo) Messages(instanceID string, q MessageQuery) ([]Message, int64, error) {
	tx := r.DB.Model(&Message{}).Where("instance_id = ?", instanceID)
	if q.Chat != "" {
		tx = tx.Where("chat = ?", q.Chat)
	}
	if q.ID != "" {
		tx = tx.Where("id = ?", q.ID)
	}
	if q.FromMe != nil {
		tx = tx.Where("from_me = ?", *q.FromMe)
	}
	if q.Before > 0 {
		tx = tx.Where("timestamp < ?", q.Before)
	}
	if q.After > 0 {
		tx = tx.Where("timestamp > ?", q.After)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	var list []Message
	err := tx.Order("timestamp DESC, id DESC").Limit(limit).Offset(max(q.Offset, 0)).Find(&list).Error
	return list, total, err
}

// OldestMessage devolve a mensagem mais antiga guardada da conversa.
func (r *Repo) OldestMessage(instanceID, chat string) (*Message, error) {
	var m Message
	if err := r.DB.Where("instance_id = ? AND chat = ?", instanceID, chat).Order("timestamp ASC").First(&m).Error; err != nil {
		return nil, ErrMessageNotFound
	}
	return &m, nil
}

// TouchChat registra uma mensagem na conversa (última mensagem e não lidas).
func (r *Repo) TouchChat(instanceID, jid, name string, m *Message, incoming bool) error {
	return r.DB.Transaction(func(tx *gorm.DB) error {
		var c Chat
		err := tx.Where("instance_id = ? AND jid = ?", instanceID, jid).First(&c).Error
		if err != nil {
			c = Chat{InstanceID: instanceID, JID: jid}
		}
		if name != "" {
			c.Name = name
		}
		if m != nil && m.Timestamp >= c.LastTimestamp {
			c.LastMessageID, c.LastText, c.LastFromMe, c.LastSender, c.LastTimestamp = m.ID, m.Text, m.FromMe, m.Sender, m.Timestamp
			if incoming {
				c.Unread++
				c.Archived = false
			}
		}
		return tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&c).Error
	})
}

// SaveChat grava a conversa inteira (usado pela sincronização do celular).
func (r *Repo) SaveChat(c *Chat) error {
	var old Chat
	if err := r.DB.Where("instance_id = ? AND jid = ?", c.InstanceID, c.JID).First(&old).Error; err == nil {
		if old.LastTimestamp > c.LastTimestamp {
			c.LastMessageID, c.LastText, c.LastFromMe, c.LastSender, c.LastTimestamp =
				old.LastMessageID, old.LastText, old.LastFromMe, old.LastSender, old.LastTimestamp
		}
		if c.Name == "" {
			c.Name = old.Name
		}
	}
	return r.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(c).Error
}

// UpdateChat muda campos de uma conversa já conhecida.
func (r *Repo) UpdateChat(instanceID, jid string, fields map[string]any) error {
	return r.DB.Model(&Chat{}).Where("instance_id = ? AND jid = ?", instanceID, jid).Updates(fields).Error
}

// ChatByJID busca uma conversa.
func (r *Repo) ChatByJID(instanceID, jid string) (*Chat, error) {
	var c Chat
	if err := r.DB.Where("instance_id = ? AND jid = ?", instanceID, jid).First(&c).Error; err != nil {
		return nil, ErrNotFound
	}
	return &c, nil
}

// Chats lista as conversas, as mais recentes primeiro.
func (r *Repo) Chats(instanceID string, limit, offset int) ([]Chat, int64, error) {
	tx := r.DB.Model(&Chat{}).Where("instance_id = ?", instanceID)
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	var list []Chat
	err := tx.Order("pinned DESC, last_timestamp DESC").Limit(limit).Offset(max(offset, 0)).Find(&list).Error
	return list, total, err
}

// SavePollVote grava o voto mais recente da pessoa.
func (r *Repo) SavePollVote(v *PollVote) error {
	return r.DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(v).Error
}

// PollVotes lista os votos de uma enquete.
func (r *Repo) PollVotes(instanceID, pollID string) ([]PollVote, error) {
	var list []PollVote
	err := r.DB.Where("instance_id = ? AND poll_id = ?", instanceID, pollID).Order("timestamp").Find(&list).Error
	return list, err
}

// PurgeMessages apaga do histórico as mensagens mais antigas que before.
func (r *Repo) PurgeMessages(before time.Time) (int64, error) {
	res := r.DB.Where("timestamp < ?", before.Unix()).Delete(&Message{})
	return res.RowsAffected, res.Error
}

// deleteHistory apaga o histórico de uma instância.
func deleteHistory(tx *gorm.DB, instanceID string) {
	tx.Where("instance_id = ?", instanceID).Delete(&Message{})
	tx.Where("instance_id = ?", instanceID).Delete(&Chat{})
	tx.Where("instance_id = ?", instanceID).Delete(&PollVote{})
}
