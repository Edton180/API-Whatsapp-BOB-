// API WhatsApp BOB - Desenvolvido por Edton Braz.

package calls

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

// bridge é a conexão WebRTC com o navegador de um atendente. A sinalização não é "trickle":
// o navegador manda a oferta já com todos os candidatos e a BOB responde do mesmo jeito, o que
// basta uma requisição HTTP. Candidatos extras ainda podem chegar por /call/ice.
//
// Na transferência, só a bridge muda: o áudio e o vídeo do WhatsApp continuam ligados à
// chamada e passam a ir para a conexão do novo atendente.
type bridge struct {
	pc         *webrtc.PeerConnection
	audioMime  string
	audioTrack *webrtc.TrackLocalStaticSample
	videoTrack *webrtc.TrackLocalStaticSample

	mu        sync.Mutex
	videoSSRC webrtc.SSRC
	closed    bool
	connected bool
	onConnect func()
	onFailed  func()
	onAudio   func([]float32)
	onVideo   func([]byte)
}

// bridgeHooks liga a bridge à chamada.
type bridgeHooks struct {
	audio     func(samples []float32) // áudio do navegador já em PCM de 16 kHz
	video     func(accessUnit []byte) // vídeo do navegador em H.264 Annex-B
	connected func()
	failed    func()
}

var errNoAudio = errors.New("a oferta do navegador não tem áudio")

// offerAudioMime escolhe o codec de áudio: G.722 se o navegador oferecer, senão PCMU.
func offerAudioMime(sdp string) (string, error) {
	low := strings.ToLower(sdp)
	if !strings.Contains(low, "m=audio") {
		return "", errNoAudio
	}
	if strings.Contains(low, "g722/8000") {
		return webrtc.MimeTypeG722, nil
	}
	if strings.Contains(low, "pcmu/8000") {
		return webrtc.MimeTypePCMU, nil
	}
	return "", errors.New("o navegador não ofereceu G.722 nem PCMU")
}

// newBridge recebe a oferta SDP do navegador e devolve a bridge com a resposta pronta.
func newBridge(api *webrtc.API, servers []ICEServer, offer string, h bridgeHooks) (*bridge, string, error) {
	mime, err := offerAudioMime(offer)
	if err != nil {
		return nil, "", err
	}
	conf := webrtc.Configuration{}
	for _, s := range servers {
		conf.ICEServers = append(conf.ICEServers, s.pion())
	}
	pc, err := api.NewPeerConnection(conf)
	if err != nil {
		return nil, "", err
	}
	b := &bridge{pc: pc, audioMime: mime, onConnect: h.connected, onFailed: h.failed, onAudio: h.audio, onVideo: h.video}
	fail := func(err error) (*bridge, string, error) {
		_ = pc.Close()
		return nil, "", err
	}

	if err := pc.SetRemoteDescription(webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offer}); err != nil {
		return fail(fmt.Errorf("oferta SDP inválida: %w", err))
	}
	b.audioTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: mime, ClockRate: 8000, Channels: 1}, "audio", "bob")
	if err != nil {
		return fail(err)
	}
	sender, err := pc.AddTrack(b.audioTrack)
	if err != nil {
		return fail(err)
	}
	go drainRTCP(sender, nil)

	// Vídeo: o navegador sempre pode mandar uma linha de vídeo (mesmo sem câmera ligada), assim
	// a câmera liga e desliga no meio da chamada sem renegociar.
	if strings.Contains(strings.ToLower(offer), "m=video") {
		b.videoTrack, err = webrtc.NewTrackLocalStaticSample(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264Fmtp}, "video", "bob")
		if err != nil {
			return fail(err)
		}
		vs, err := pc.AddTrack(b.videoTrack)
		if err != nil {
			return fail(err)
		}
		go drainRTCP(vs, nil)
	}

	pc.OnTrack(b.onTrack)
	pc.OnConnectionStateChange(b.onState)

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return fail(err)
	}
	gathered := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		return fail(err)
	}
	select {
	case <-gathered:
	case <-time.After(5 * time.Second):
		// Responde com os candidatos que já tiver: o resto chega pelo STUN/TURN do navegador.
	}
	local := pc.LocalDescription()
	if local == nil {
		return fail(errors.New("não consegui montar a resposta SDP"))
	}

	// Sem conexão em 30 s (rede bloqueada, TURN faltando), a chamada não fica presa.
	time.AfterFunc(30*time.Second, func() {
		b.mu.Lock()
		stuck := !b.connected && !b.closed
		b.mu.Unlock()
		if stuck && b.onFailed != nil {
			b.onFailed()
		}
	})
	return b, local.SDP, nil
}

func drainRTCP(s *webrtc.RTPSender, onPLI func()) {
	for {
		pkts, _, err := s.ReadRTCP()
		if err != nil {
			return
		}
		for _, p := range pkts {
			switch p.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				if onPLI != nil {
					onPLI()
				}
			}
		}
	}
}

func (b *bridge) onState(st webrtc.PeerConnectionState) {
	switch st {
	case webrtc.PeerConnectionStateConnected:
		b.mu.Lock()
		first := !b.connected
		b.connected = true
		b.mu.Unlock()
		if first && b.onConnect != nil {
			b.onConnect()
		}
	case webrtc.PeerConnectionStateFailed:
		b.mu.Lock()
		closed := b.closed
		b.mu.Unlock()
		if !closed && b.onFailed != nil {
			b.onFailed()
		}
	}
}

func (b *bridge) onTrack(t *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
	switch t.Kind() {
	case webrtc.RTPCodecTypeAudio:
		codec := newAudioCodec(t.Codec().MimeType)
		for {
			pkt, _, err := t.ReadRTP()
			if err != nil {
				return
			}
			if len(pkt.Payload) == 0 || b.onAudio == nil {
				continue
			}
			b.onAudio(toFloat(codec.decode(pkt.Payload)))
		}
	case webrtc.RTPCodecTypeVideo:
		b.mu.Lock()
		b.videoSSRC = t.SSRC()
		b.mu.Unlock()
		b.RequestKeyframe()
		sb := samplebuilder.New(256, &codecs.H264Packet{}, t.Codec().ClockRate)
		for {
			pkt, _, err := t.ReadRTP()
			if err != nil {
				return
			}
			sb.Push(pkt)
			for s := sb.Pop(); s != nil; s = sb.Pop() {
				if b.onVideo != nil && len(s.Data) > 0 {
					b.onVideo(s.Data)
				}
			}
		}
	}
}

// RequestKeyframe pede ao navegador um quadro completo (o WhatsApp pediu, ou o vídeo começou).
func (b *bridge) RequestKeyframe() {
	b.mu.Lock()
	ssrc := b.videoSSRC
	b.mu.Unlock()
	if ssrc != 0 {
		_ = b.pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(ssrc)}})
	}
}

// WriteSample implementa sampleWriter: áudio do WhatsApp para o navegador.
func (b *bridge) WriteSample(s media.Sample) error { return b.audioTrack.WriteSample(s) }

// WriteVideo manda ao navegador um quadro H.264 recebido do WhatsApp.
func (b *bridge) WriteVideo(accessUnit []byte) error {
	if b.videoTrack == nil {
		return nil
	}
	return b.videoTrack.WriteSample(media.Sample{Data: accessUnit, Duration: time.Second / 30})
}

// AddCandidate recebe um candidato ICE extra do navegador (sinalização trickle opcional).
func (b *bridge) AddCandidate(c webrtc.ICECandidateInit) error { return b.pc.AddICECandidate(c) }

// Close encerra a conexão com o navegador. Pode ser chamada mais de uma vez.
func (b *bridge) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.mu.Unlock()
	_ = b.pc.Close()
}
