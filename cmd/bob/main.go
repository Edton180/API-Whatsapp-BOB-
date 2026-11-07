// API WhatsApp BOB - Desenvolvido por Edton Braz.
//
// Servidor da API WhatsApp BOB: motor próprio sobre a biblioteca whatsmeow, painel web,
// webhooks, WebSocket e integrações com Chatwoot, Typebot e N8N.
package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edton180/api-whatsapp-bob/internal/api"
	"github.com/edton180/api-whatsapp-bob/internal/config"
	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/events"
	"github.com/edton180/api-whatsapp-bob/internal/integrations"
	"github.com/edton180/api-whatsapp-bob/internal/media"
	"github.com/edton180/api-whatsapp-bob/internal/store"
	"github.com/edton180/api-whatsapp-bob/internal/version"
)

func main() {
	log.SetFlags(log.LstdFlags)
	cfg := config.Load()
	if cfg.APIKey == "" {
		log.Fatal("defina GLOBAL_API_KEY (chave global da API) no .env ou no ambiente")
	}
	log.Printf("%s %s iniciando (motor: %s)", version.Name, version.Current(), version.Engine)

	db, err := store.Open(cfg.UsersDB, cfg.DataDir)
	if err != nil {
		log.Fatal(err)
	}
	repo := &store.Repo{DB: db}
	disp := events.NewDispatcher()
	eng, err := engine.New(cfg, repo, disp)
	if err != nil {
		log.Fatal(err)
	}

	intStore, err := integrations.NewGormStore(db)
	if err != nil {
		log.Fatal(err)
	}
	mgr := &integrations.Manager{Store: intStore, Sender: sender{eng}, Instance: repo.ByID}
	integrations.SetDefault(mgr)
	disp.AddHook(func(instanceID string, body []byte) {
		if !bytes.Contains(body[:min(len(body), 64)], []byte(`"event":"Message"`)) {
			return
		}
		if inst, err := repo.ByID(instanceID); err == nil {
			integrations.Dispatch(inst, body)
		}
	})

	srv := &api.Server{Engine: eng, Repo: repo, Integrations: mgr}
	httpSrv := &http.Server{Addr: ":" + cfg.Port, Handler: srv.Router(), ReadHeaderTimeout: 15 * time.Second}

	go eng.Start()
	go func() {
		log.Printf("API no ar em http://0.0.0.0:%s  (painel: /painel/  documentação: /swagger/index.html)", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Print("encerrando: as sessões continuam salvas e voltam sozinhas no próximo início")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	eng.Stop()
}

// sender liga as integrações ao motor.
type sender struct{ e *engine.Engine }

func (s sender) session(inst *store.Instance) *engine.Session { return s.e.Session(inst) }

func (s sender) Text(inst *store.Instance, number, text string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	sess := s.session(inst)
	to, err := sess.ResolveJID(ctx, number)
	if err != nil {
		return "", err
	}
	res, err := sess.Send(ctx, to, engine.Text(text), engine.SendOptions{})
	return messageID(res), err
}

func (s sender) Media(inst *store.Instance, number, url, mediaType, caption string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	sess := s.session(inst)
	to, err := sess.ResolveJID(ctx, number)
	if err != nil {
		return "", err
	}
	data, mime, err := media.Load(ctx, url)
	if err != nil {
		return "", err
	}
	if mediaType == "sticker" {
		m, err := sess.Sticker(ctx, data)
		if err != nil {
			return "", err
		}
		res, err := sess.Send(ctx, to, m, engine.SendOptions{})
		return messageID(res), err
	}
	m, err := sess.Media(ctx, engine.MediaInput{Data: data, Mime: mime, Type: mediaType, Caption: caption})
	if err != nil {
		return "", err
	}
	res, err := sess.Send(ctx, to, m, engine.SendOptions{})
	return messageID(res), err
}

func messageID(res map[string]any) string {
	if info, ok := res["Info"].(map[string]any); ok {
		if id, ok := info["ID"]; ok {
			if s, ok := id.(string); ok {
				return s
			}
			return toString(id)
		}
	}
	return ""
}
