// API WhatsApp BOB - Desenvolvido por Edton Braz.

package calls

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"

	"github.com/edton180/api-whatsapp-bob/internal/config"
)

// fakeBrowser é um RTCPeerConnection como o do navegador: áudio G.722 e uma linha de vídeo H.264.
type fakeBrowser struct {
	pc    *webrtc.PeerConnection
	audio *webrtc.TrackLocalStaticSample
	video *webrtc.TrackLocalStaticSample

	mu       sync.Mutex
	heard    []int16
	gotVideo bool
}

func newFakeBrowser(t *testing.T, mime string) *fakeBrowser {
	t.Helper()
	m := &webrtc.MediaEngine{}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: mime, ClockRate: 8000, Channels: 1}, PayloadType: map[string]webrtc.PayloadType{webrtc.MimeTypeG722: 9, webrtc.MimeTypePCMU: 0}[mime]}, webrtc.RTPCodecTypeAudio); err != nil {
		t.Fatal(err)
	}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264Fmtp}, PayloadType: 102}, webrtc.RTPCodecTypeVideo); err != nil {
		t.Fatal(err)
	}
	se := webrtc.SettingEngine{}
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4})
	pc, err := webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(se)).NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	b := &fakeBrowser{pc: pc}
	b.audio, _ = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: mime, ClockRate: 8000, Channels: 1}, "mic", "navegador")
	b.video, _ = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000}, "cam", "navegador")
	if _, err := pc.AddTrack(b.audio); err != nil {
		t.Fatal(err)
	}
	if _, err := pc.AddTrack(b.video); err != nil {
		t.Fatal(err)
	}
	pc.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		codec := newAudioCodec(tr.Codec().MimeType)
		for {
			pkt, _, err := tr.ReadRTP()
			if err != nil {
				return
			}
			b.mu.Lock()
			if tr.Kind() == webrtc.RTPCodecTypeAudio {
				b.heard = append(b.heard, codec.decode(pkt.Payload)...)
			} else {
				b.gotVideo = true
			}
			b.mu.Unlock()
		}
	})
	return b
}

func (b *fakeBrowser) offer(t *testing.T) string {
	t.Helper()
	o, err := b.pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	done := webrtc.GatheringCompletePromise(b.pc)
	if err := b.pc.SetLocalDescription(o); err != nil {
		t.Fatal(err)
	}
	<-done
	return b.pc.LocalDescription().SDP
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("tempo esgotado esperando %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A ponte completa com um navegador: conecta, leva o áudio nos dois sentidos e passa o vídeo
// H.264 do navegador inteiro (Annex-B) para o lado do WhatsApp.
func TestBridgeWithBrowser(t *testing.T) {
	for _, mime := range []string{webrtc.MimeTypeG722, webrtc.MimeTypePCMU} {
		t.Run(mime, func(t *testing.T) {
			api, closer, err := newAPI(config.WebRTC{})
			if err != nil {
				t.Fatal(err)
			}
			defer closer()
			browser := newFakeBrowser(t, mime)
			defer browser.pc.Close()

			var mu sync.Mutex
			var fromBrowser []float32
			var units [][]byte
			connected := make(chan struct{}, 1)
			br, answer, err := newBridge(api, nil, browser.offer(t), bridgeHooks{
				audio:     func(s []float32) { mu.Lock(); fromBrowser = append(fromBrowser, s...); mu.Unlock() },
				video:     func(au []byte) { mu.Lock(); units = append(units, append([]byte(nil), au...)); mu.Unlock() },
				connected: func() { connected <- struct{}{} },
				failed:    func() { t.Error("a conexão não deveria falhar") },
			})
			if err != nil {
				t.Fatal(err)
			}
			defer br.Close()
			if br.audioMime != mime {
				t.Fatalf("codec %s, esperado %s", br.audioMime, mime)
			}
			if err := browser.pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answer}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-connected:
			case <-time.After(10 * time.Second):
				t.Fatal("a ponte não conectou")
			}

			// Navegador -> BOB: 1 s de tom no codec negociado.
			enc := newAudioCodec(mime)
			tn := tone(1000, 16000, 8000)
			idr := []byte{0, 0, 0, 1, 0x67, 0x42, 0xe0, 0x1f, 0, 0, 0, 1, 0x68, 0xce, 0x3c, 0x80, 0, 0, 0, 1, 0x65, 0x88, 0x84, 0x21, 0xa0}
			for i := 0; i < len(tn); i += packetSamp {
				_ = browser.audio.WriteSample(media.Sample{Data: enc.encode(tn[i : i+packetSamp]), Duration: 20 * time.Millisecond})
				if i%(packetSamp*3) == 0 {
					_ = browser.video.WriteSample(media.Sample{Data: idr, Duration: time.Second / 30})
				}
				time.Sleep(2 * time.Millisecond)
			}
			waitFor(t, "áudio do navegador", func() bool { mu.Lock(); defer mu.Unlock(); return len(fromBrowser) >= 8000 })
			mu.Lock()
			var energy float64
			for _, v := range fromBrowser {
				energy += float64(v * v)
			}
			rms := energy / float64(len(fromBrowser))
			mu.Unlock()
			if rms < 0.01 {
				t.Fatalf("áudio do navegador chegou mudo (rms² %.4f)", rms)
			}
			waitFor(t, "vídeo do navegador", func() bool { mu.Lock(); defer mu.Unlock(); return len(units) > 2 })
			mu.Lock()
			if !bytes.Contains(units[1], []byte{0x65, 0x88, 0x84, 0x21, 0xa0}) || !bytes.HasPrefix(units[1], []byte{0, 0, 0, 1}) {
				t.Fatalf("quadro H.264 incompleto: %x", units[1])
			}
			mu.Unlock()

			// BOB -> navegador: o áudio do WhatsApp vira pacotes de 20 ms no codec do navegador.
			sink := &browserSink{}
			sink.setTrack(br, newAudioCodec(br.audioMime))
			for range 20 {
				_ = sink.WriteFrame(toFloat(tone(500, 960, 8000)))
				time.Sleep(10 * time.Millisecond)
			}
			for range 10 {
				_ = br.WriteVideo(idr)
			}
			waitFor(t, "áudio no navegador", func() bool {
				browser.mu.Lock()
				defer browser.mu.Unlock()
				return len(browser.heard) >= 16000 && browser.gotVideo
			})
			browser.mu.Lock()
			var peak int16
			for _, v := range browser.heard {
				peak = max(peak, v)
			}
			browser.mu.Unlock()
			if peak < 2000 {
				t.Fatalf("navegador ouviu silêncio (pico %d)", peak)
			}
		})
	}
}

// Uma oferta que não tem áudio não abre conexão.
func TestBridgeRejectsBadOffer(t *testing.T) {
	api, closer, err := newAPI(config.WebRTC{})
	if err != nil {
		t.Fatal(err)
	}
	defer closer()
	if _, _, err := newBridge(api, nil, "v=0\r\nm=video 9 UDP/TLS/RTP/SAVPF 102\r\n", bridgeHooks{}); err == nil {
		t.Fatal("deveria recusar")
	}
}
