// API WhatsApp BOB - Desenvolvido por Edton Braz.

package calls

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/purpshell/meowcaller"

	"github.com/edton180/api-whatsapp-bob/internal/config"
)

// A credencial TURN é temporária e conferível pelo servidor TURN só com o segredo compartilhado.
func TestICEServersTURNCredential(t *testing.T) {
	cfg := config.WebRTC{
		ICEServers: []string{"stun:stun.exemplo.com:3478", "turn:turn.exemplo.com:3478?transport=udp", "turns:turn.exemplo.com:5349", "lixo"},
		TURNSecret: "segredo", TURNTTL: 300,
	}
	now := time.Unix(1_800_000_000, 0)
	list := ICEServers(cfg, "Ana:Paula", now)
	if len(list) != 2 || len(list[0].URLs) != 1 || len(list[1].URLs) != 2 {
		t.Fatalf("%+v", list)
	}
	turn := list[1]
	if turn.Username != "1800000300:Ana-Paula" {
		t.Fatalf("usuário %q", turn.Username)
	}
	mac := hmac.New(sha1.New, []byte("segredo"))
	mac.Write([]byte(turn.Username))
	if turn.Credential != base64.StdEncoding.EncodeToString(mac.Sum(nil)) {
		t.Fatal("credencial não confere com o HMAC do segredo")
	}
	if strings.Contains(turn.Credential, "segredo") {
		t.Fatal("o segredo vazou")
	}
	// Sem segredo, o TURN não é oferecido (não há como gerar credencial).
	cfg.TURNSecret = ""
	if l := ICEServers(cfg, "x", now); len(l) != 1 {
		t.Fatalf("%+v", l)
	}
}

func TestLiveSourceNeverBlocks(t *testing.T) {
	l := &liveSource{}
	f, err := l.ReadFrame()
	if err != nil || len(f) != meowcaller.FrameSamples {
		t.Fatalf("quadro vazio: %v %d", err, len(f))
	}
	l.push(make([]float32, 500))
	for i := range 500 {
		l.buf[i] = 0.5
	}
	f, _ = l.ReadFrame()
	if f[0] != 0.5 || f[499] != 0.5 || f[500] != 0 {
		t.Fatal("áudio parcial deveria vir completado com silêncio")
	}
	l.push(make([]float32, rate)) // 1 s de atraso acumulado
	if len(l.buf) != maxBuffered {
		t.Fatalf("buffer %d, esperado no máximo %d", len(l.buf), maxBuffered)
	}
	l.setMuted(true)
	l.push([]float32{1, 1, 1})
	if f, _ = l.ReadFrame(); f[0] != 0 {
		t.Fatal("microfone desligado deveria mandar silêncio")
	}
	_ = l.Close()
	if _, err := l.ReadFrame(); err != nil {
		t.Fatal("fechado não pode dar erro (o motor pararia o envio)")
	}
}

type captureWriter struct{ samples []media.Sample }

func (c *captureWriter) WriteSample(s media.Sample) error {
	c.samples = append(c.samples, s)
	return nil
}

func TestBrowserSinkPackets(t *testing.T) {
	w := &captureWriter{}
	b := &browserSink{track: w, codec: newAudioCodec("audio/G722")}
	_ = b.WriteFrame(make([]float32, meowcaller.FrameSamples)) // 60 ms
	if len(w.samples) != 3 || len(w.samples[0].Data) != 160 || w.samples[0].Duration != 20*time.Millisecond {
		t.Fatalf("esperado 3 pacotes G.722 de 20 ms: %d", len(w.samples))
	}
	b.setTrack(w, newAudioCodec("audio/PCMU"))
	_ = b.WriteFrame(make([]float32, meowcaller.FrameSamples))
	if len(w.samples) != 6 || len(w.samples[5].Data) != 160 {
		t.Fatalf("PCMU: %d pacotes", len(w.samples))
	}
	_ = b.Close()
	_ = b.WriteFrame(make([]float32, meowcaller.FrameSamples))
	if len(w.samples) != 6 {
		t.Fatal("fechado não deveria mandar nada")
	}
}

func TestPCMURoundTrip(t *testing.T) {
	c := &pcmuCodec{}
	in := make([]int16, 3200)
	for i := range in {
		in[i] = int16(6000 * math.Sin(2*math.Pi*400*float64(i)/16000))
	}
	out := c.decode(c.encode(in))
	if len(out) != len(in) {
		t.Fatalf("%d amostras", len(out))
	}
	if s, _ := snr(in, out); s < 15 {
		t.Fatalf("SNR %.1f dB", s)
	}
}

func TestOfferAudioMime(t *testing.T) {
	cases := map[string]string{
		"m=audio 9 UDP/TLS/RTP/SAVPF 111 9 0\r\na=rtpmap:9 G722/8000\r\na=rtpmap:0 PCMU/8000": "audio/G722",
		"m=audio 9 UDP/TLS/RTP/SAVPF 0\r\na=rtpmap:0 PCMU/8000":                               "audio/PCMU",
	}
	for sdp, want := range cases {
		if got, err := offerAudioMime(sdp); err != nil || got != want {
			t.Fatalf("%q: %v %v", want, got, err)
		}
	}
	if _, err := offerAudioMime("m=video 9"); err == nil {
		t.Fatal("sem áudio deveria falhar")
	}
	if _, err := offerAudioMime("m=audio 9 x 111\r\na=rtpmap:111 opus/48000/2"); err == nil {
		t.Fatal("só Opus deveria falhar")
	}
}
