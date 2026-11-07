package evolutionapi

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_repository "github.com/evolution-foundation/evolution-go/pkg/instance/repository"
	"github.com/evolution-foundation/evolution-go/pkg/internal/event_types"
)

type fakeRepo struct {
	instance_repository.InstanceRepository
	byName map[string]*instance_model.Instance
}

func (f fakeRepo) GetInstanceByName(n string) (*instance_model.Instance, error) {
	if i, ok := f.byName[n]; ok {
		return i, nil
	}
	return nil, errors.New("not found")
}

func (f fakeRepo) GetInstanceByID(id string) (*instance_model.Instance, error) {
	for _, i := range f.byName {
		if i.Id == id {
			return i, nil
		}
	}
	return nil, errors.New("not found")
}

func (f fakeRepo) GetInstanceByToken(t string) (*instance_model.Instance, error) {
	for _, i := range f.byName {
		if i.Token == t {
			return i, nil
		}
	}
	return nil, errors.New("not found")
}

func newTestHandler() *Handler {
	return New(Deps{
		Config:        &config.Config{GlobalApiKey: "global"},
		ClientPointer: map[string]*whatsmeow.Client{},
		InstanceRepository: fakeRepo{byName: map[string]*instance_model.Instance{
			"bob": {Id: "11111111-1111-1111-1111-111111111111", Name: "bob", Token: "bob-token"},
		}},
	})
}

func TestRegisterHasNoRouteConflicts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	// Every native Evolution Go route (pkg/routes/routes.go). Registering the
	// compatibility layer next to them must not panic on a route conflict.
	native := []struct{ method, path string }{
		{"GET", "/swagger/*any"},
		{"GET", "/favicon.ico"},
		{"GET", "/manager/*any"},
		{"GET", "/manager"},
		{"GET", "/server/ok"},
		{"POST", "/instance/create"},
		{"GET", "/instance/all"},
		{"GET", "/instance/info/:instanceId"},
		{"DELETE", "/instance/delete/:instanceId"},
		{"POST", "/instance/proxy/:instanceId"},
		{"DELETE", "/instance/proxy/:instanceId"},
		{"POST", "/instance/forcereconnect/:instanceId"},
		{"GET", "/instance/logs/:instanceId"},
		{"POST", "/instance/connect"},
		{"GET", "/instance/status"},
		{"GET", "/instance/qr"},
		{"POST", "/instance/pair"},
		{"POST", "/instance/disconnect"},
		{"POST", "/instance/reconnect"},
		{"DELETE", "/instance/logout"},
		{"GET", "/instance/:instanceId/advanced-settings"},
		{"PUT", "/instance/:instanceId/advanced-settings"},
		{"POST", "/send/text"},
		{"POST", "/send/link"},
		{"POST", "/send/media"},
		{"POST", "/send/poll"},
		{"POST", "/send/sticker"},
		{"POST", "/send/location"},
		{"POST", "/send/contact"},
		{"POST", "/send/button"},
		{"POST", "/send/list"},
		{"POST", "/send/carousel"},
		{"POST", "/send/status/text"},
		{"POST", "/send/status/media"},
		{"POST", "/user/info"},
		{"POST", "/user/check"},
		{"POST", "/user/avatar"},
		{"GET", "/user/contacts"},
		{"GET", "/user/privacy"},
		{"POST", "/user/privacy"},
		{"POST", "/user/block"},
		{"POST", "/user/unblock"},
		{"GET", "/user/blocklist"},
		{"POST", "/user/profilePicture"},
		{"POST", "/user/profileName"},
		{"POST", "/user/profileStatus"},
		{"POST", "/message/react"},
		{"POST", "/message/presence"},
		{"POST", "/message/markread"},
		{"POST", "/message/markplayed"},
		{"POST", "/message/downloadmedia"},
		{"POST", "/message/status"},
		{"POST", "/message/delete"},
		{"POST", "/message/edit"},
		{"POST", "/chat/pin"},
		{"POST", "/chat/unpin"},
		{"POST", "/chat/archive"},
		{"POST", "/chat/unarchive"},
		{"POST", "/chat/mute"},
		{"POST", "/chat/unmute"},
		{"POST", "/chat/history-sync"},
		{"GET", "/group/list"},
		{"POST", "/group/info"},
		{"POST", "/group/invitelink"},
		{"POST", "/group/photo"},
		{"POST", "/group/name"},
		{"POST", "/group/description"},
		{"POST", "/group/create"},
		{"POST", "/group/participant"},
		{"GET", "/group/myall"},
		{"POST", "/group/join"},
		{"POST", "/group/leave"},
		{"POST", "/group/settings"},
		{"POST", "/call/reject"},
		{"POST", "/community/create"},
		{"POST", "/community/add"},
		{"POST", "/community/remove"},
		{"POST", "/label/chat"},
		{"POST", "/label/message"},
		{"POST", "/label/edit"},
		{"GET", "/label/list"},
		{"POST", "/unlabel/chat"},
		{"POST", "/unlabel/message"},
		{"POST", "/newsletter/create"},
		{"GET", "/newsletter/list"},
		{"POST", "/newsletter/info"},
		{"POST", "/newsletter/link"},
		{"POST", "/newsletter/subscribe"},
		{"POST", "/newsletter/messages"},
		{"GET", "/polls/:pollMessageId/results"},
	}
	noop := func(c *gin.Context) {}
	for _, r := range native {
		eng.Handle(r.method, r.path, noop)
	}
	newTestHandler().Register(eng)
}

func TestRequireInstanceAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := newTestHandler()
	eng := gin.New()
	eng.GET("/settings/find/:instance", h.requireInstance, func(c *gin.Context) {
		c.String(http.StatusOK, getInstance(c).Name)
	})
	cases := []struct {
		path, key string
		want      int
	}{
		{"/settings/find/bob", "global", http.StatusOK},
		{"/settings/find/bob", "bob-token", http.StatusOK},
		{"/settings/find/11111111-1111-1111-1111-111111111111", "bob-token", http.StatusOK},
		{"/settings/find/bob", "wrong", http.StatusUnauthorized},
		{"/settings/find/bob", "", http.StatusUnauthorized},
		{"/settings/find/missing", "global", http.StatusNotFound},
		{"/settings/find/missing", "bob-token", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.key != "" {
			req.Header.Set("apikey", tc.key)
		}
		w := httptest.NewRecorder()
		eng.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s key=%q: got %d, want %d", tc.path, tc.key, w.Code, tc.want)
		}
	}
}

func TestEventMapping(t *testing.T) {
	got := toEngineEvents([]string{"MESSAGES_UPSERT", "messages.update", "PRESENCE_UPDATE", "GROUP_UPDATE", "GROUPS_UPSERT", "bogus"})
	want := []string{event_types.MESSAGE, event_types.READ_RECEIPT, event_types.PRESENCE, event_types.CHAT_PRESENCE, event_types.GROUP}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("toEngineEvents = %v, want %v", got, want)
	}
	if all := toEngineEvents([]string{"ALL"}); len(all) != len(event_types.AllEventTypes) {
		t.Fatalf("ALL expanded to %d events", len(all))
	}
	back := fromEngineEvents("MESSAGE,READ_RECEIPT")
	if strings.Join(back, ",") != "MESSAGES_UPSERT,MESSAGES_UPDATE" {
		t.Fatalf("fromEngineEvents = %v", back)
	}
}

func TestDecodeMedia(t *testing.T) {
	raw := []byte("hello world")
	for _, in := range []string{
		base64.StdEncoding.EncodeToString(raw),
		"data:text/plain;base64," + base64.StdEncoding.EncodeToString(raw),
		base64.RawStdEncoding.EncodeToString(raw),
	} {
		out, err := decodeMedia(in)
		if err != nil || string(out) != string(raw) {
			t.Errorf("decodeMedia(%q) = %q, %v", in, out, err)
		}
	}
	if _, err := decodeMedia("not base64 !!"); err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestContactVcard(t *testing.T) {
	v := contactCard{FullName: "Bob", PhoneNumber: "+55 11 99999-0000", Organization: "Acme", Email: "a@b.c"}.vcard()
	for _, want := range []string{"FN:Bob", "ORG:Acme;", "EMAIL:a@b.c", "waid=5511999990000:+55 11 99999-0000", "END:VCARD"} {
		if !strings.Contains(v, want) {
			t.Errorf("vcard missing %q:\n%s", want, v)
		}
	}
}

func TestMessageTypeOf(t *testing.T) {
	if got := messageTypeOf(map[string]interface{}{"messageContextInfo": 1, "imageMessage": 2}); got != "imageMessage" {
		t.Fatalf("got %s", got)
	}
}

func TestInviteCodeFrom(t *testing.T) {
	if got := inviteCodeFrom("https://chat.whatsapp.com/AbC123"); got != "AbC123" {
		t.Fatalf("got %s", got)
	}
}
