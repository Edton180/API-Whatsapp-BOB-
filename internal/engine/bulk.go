// API WhatsApp BOB - Desenvolvido por Edton Braz.

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Tarefas em massa: a BOB percorre uma lista (adicionar pessoas a um grupo, enviar a mesma
// mensagem para vários números, criar vários grupos) um item por vez, com intervalo entre eles.
// A tarefa pode ser pausada, retomada e cancelada, e pausa sozinha quando o WhatsApp limita o
// número. As tarefas ficam na memória: reiniciar a BOB encerra as que estavam rodando.

// Tipos de tarefa.
const (
	BulkGroupAdd    = "groupAdd"
	BulkSend        = "send"
	BulkGroupCreate = "groupCreate"
)

// Limites por tipo: intervalo mínimo (segundos) e quantidade máxima de itens.
var bulkLimits = map[string]struct{ minInterval, maxItems int }{
	BulkGroupAdd:    {10, 1000},
	BulkSend:        {3, 5000},
	BulkGroupCreate: {30, 50},
}

// BulkItem é um item da lista e o que aconteceu com ele.
type BulkItem struct {
	Target string `json:"target"`
	Status string `json:"status"` // pending, done, failed, cancelled
	Detail string `json:"detail,omitempty"`
	At     int64  `json:"at,omitempty"`
}

// BulkJob é uma tarefa em massa.
type BulkJob struct {
	ID         string     `json:"id"`
	InstanceID string     `json:"instanceId"`
	Kind       string     `json:"kind"`
	Label      string     `json:"label,omitempty"`
	Status     string     `json:"status"` // running, paused, completed, cancelled
	Interval   int        `json:"intervalSeconds"`
	Total      int        `json:"total"`
	Done       int        `json:"done"`
	Failed     int        `json:"failed"`
	Pending    int        `json:"pending"`
	LastError  string     `json:"lastError,omitempty"`
	CreatedAt  int64      `json:"createdAt"`
	FinishedAt int64      `json:"finishedAt,omitempty"`
	Items      []BulkItem `json:"items,omitempty"`
}

// BulkStep executa um item e devolve um detalhe para o relatório.
type BulkStep func(ctx context.Context, target string) (detail string, err error)

// bulkFailure marca um item que falhou sem ser erro da tarefa (ex.: a pessoa recusou).
type bulkFailure struct{ detail string }

func (e bulkFailure) Error() string { return e.detail }

// BulkFail devolve o erro que marca só o item como falho.
func BulkFail(detail string) error { return bulkFailure{detail} }

var rateLimited = regexp.MustCompile(`(?i)rate-overlimit|too many|\b429\b`)

type bulkState struct {
	job    BulkJob
	step   BulkStep
	cancel context.CancelFunc
	wake   chan struct{}
}

// Bulk guarda e executa as tarefas em massa.
type Bulk struct {
	mu    sync.Mutex
	jobs  map[string]*bulkState
	sleep func(ctx context.Context, d time.Duration) error
}

func newBulk() *Bulk { return &Bulk{jobs: map[string]*bulkState{}, sleep: pause} }

// Bulk devolve o gerenciador de tarefas em massa do motor.
func (e *Engine) Bulk() *Bulk {
	e.bulkOnce.Do(func() { e.bulk = newBulk() })
	return e.bulk
}

// ErrBulkNotFound é devolvido quando a tarefa não existe na instância.
var ErrBulkNotFound = errors.New("tarefa não encontrada")

// Start cria e inicia uma tarefa.
func (b *Bulk) Start(instanceID, kind, label string, targets []string, interval int, step BulkStep) (*BulkJob, error) {
	lim, ok := bulkLimits[kind]
	if !ok {
		return nil, errors.New("tipo de tarefa inválido")
	}
	seen := map[string]bool{}
	items := make([]BulkItem, 0, len(targets))
	for _, t := range targets {
		t = strings.TrimSpace(t)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		items = append(items, BulkItem{Target: t, Status: "pending"})
	}
	if len(items) == 0 {
		return nil, errors.New("a lista está vazia")
	}
	if len(items) > lim.maxItems {
		return nil, fmt.Errorf("no máximo %d itens por tarefa", lim.maxItems)
	}
	if interval < lim.minInterval {
		interval = lim.minInterval
	}
	if interval > 600 {
		interval = 600
	}
	idb := make([]byte, 8)
	_, _ = rand.Read(idb)
	ctx, cancel := context.WithCancel(context.Background())
	st := &bulkState{
		job: BulkJob{ID: hex.EncodeToString(idb), InstanceID: instanceID, Kind: kind, Label: label, Status: "running",
			Interval: interval, Total: len(items), Pending: len(items), CreatedAt: time.Now().Unix(), Items: items},
		step: step, cancel: cancel, wake: make(chan struct{}, 1),
	}
	b.mu.Lock()
	for _, other := range b.jobs {
		if other.job.InstanceID == instanceID && other.job.Kind == kind && (other.job.Status == "running" || other.job.Status == "paused") {
			b.mu.Unlock()
			cancel()
			return nil, errors.New("já existe uma tarefa deste tipo em andamento nesta instância; espere terminar ou cancele")
		}
	}
	b.jobs[st.job.ID] = st
	b.mu.Unlock()
	go b.run(ctx, st)
	return b.snapshot(st, true), nil
}

func (b *Bulk) run(ctx context.Context, st *bulkState) {
	for i := 0; i < len(st.job.Items); i++ {
		// espera enquanto estiver pausada
		for {
			b.mu.Lock()
			status := st.job.Status
			b.mu.Unlock()
			if status != "paused" {
				break
			}
			select {
			case <-ctx.Done():
			case <-st.wake:
			}
			if ctx.Err() != nil {
				break
			}
		}
		if ctx.Err() != nil {
			break
		}
		detail, err := st.step(ctx, st.job.Items[i].Target)
		b.mu.Lock()
		it := &st.job.Items[i]
		it.At = time.Now().Unix()
		if err != nil && (ctx.Err() != nil || st.job.Status == "cancelled") {
			// cancelada enquanto o item rodava: o item fica como estava e a tarefa termina cancelada
			b.mu.Unlock()
			break
		}
		var soft bulkFailure
		switch {
		case err == nil:
			it.Status, it.Detail = "done", detail
			st.job.Done++
			st.job.Pending--
		case errors.As(err, &soft):
			it.Status, it.Detail = "failed", soft.detail
			st.job.Failed++
			st.job.Pending--
		case rateLimited.MatchString(err.Error()), errors.Is(err, ErrNotLogged), errors.Is(err, ErrNotConnected):
			// não gasta a lista: o item volta a ficar pendente e a tarefa pausa
			st.job.Status = "paused"
			st.job.LastError = "tarefa pausada: " + err.Error()
			b.mu.Unlock()
			i--
			continue
		default:
			it.Status, it.Detail = "failed", err.Error()
			st.job.Failed++
			st.job.Pending--
			st.job.LastError = err.Error()
		}
		last := i == len(st.job.Items)-1
		wait := time.Duration(float64(st.job.Interval) * (0.7 + 0.6*mrand.Float64()) * float64(time.Second))
		b.mu.Unlock()
		if !last {
			if b.sleep(ctx, wait) != nil {
				break
			}
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if st.job.Status == "cancelled" {
		for i := range st.job.Items {
			if st.job.Items[i].Status == "pending" {
				st.job.Items[i].Status = "cancelled"
			}
		}
	} else {
		st.job.Status = "completed"
	}
	st.job.FinishedAt = time.Now().Unix()
	st.cancel()
}

func (b *Bulk) snapshot(st *bulkState, items bool) *BulkJob {
	b.mu.Lock()
	defer b.mu.Unlock()
	j := st.job
	if items {
		j.Items = append([]BulkItem(nil), st.job.Items...)
	} else {
		j.Items = nil
	}
	return &j
}

func (b *Bulk) find(instanceID, id string) (*bulkState, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	st, ok := b.jobs[id]
	if !ok || st.job.InstanceID != instanceID {
		return nil, ErrBulkNotFound
	}
	return st, nil
}

// Get devolve a tarefa com os itens.
func (b *Bulk) Get(instanceID, id string) (*BulkJob, error) {
	st, err := b.find(instanceID, id)
	if err != nil {
		return nil, err
	}
	return b.snapshot(st, true), nil
}

// List devolve as tarefas da instância, da mais nova para a mais antiga, sem os itens.
func (b *Bulk) List(instanceID string) []*BulkJob {
	b.mu.Lock()
	var sts []*bulkState
	for _, st := range b.jobs {
		if st.job.InstanceID == instanceID {
			sts = append(sts, st)
		}
	}
	b.mu.Unlock()
	out := make([]*BulkJob, 0, len(sts))
	for _, st := range sts {
		out = append(out, b.snapshot(st, false))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out
}

// Control pausa, retoma ou cancela a tarefa.
func (b *Bulk) Control(instanceID, id, action string) (*BulkJob, error) {
	st, err := b.find(instanceID, id)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	switch {
	case st.job.Status == "completed" || st.job.Status == "cancelled":
		b.mu.Unlock()
		return nil, errors.New("a tarefa já terminou")
	case action == "pause" && st.job.Status == "running":
		st.job.Status = "paused"
	case action == "resume" && st.job.Status == "paused":
		st.job.Status = "running"
		st.job.LastError = ""
		select {
		case st.wake <- struct{}{}:
		default:
		}
	case action == "cancel":
		st.job.Status = "cancelled"
		st.cancel()
	case action != "pause" && action != "resume":
		b.mu.Unlock()
		return nil, errors.New("ação inválida: use pause, resume ou cancel")
	}
	b.mu.Unlock()
	return b.snapshot(st, false), nil
}

// CancelInstance cancela as tarefas de uma instância (logout ou exclusão).
func (b *Bulk) CancelInstance(instanceID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, st := range b.jobs {
		if st.job.InstanceID == instanceID && (st.job.Status == "running" || st.job.Status == "paused") {
			st.job.Status = "cancelled"
			st.cancel()
		}
	}
}
