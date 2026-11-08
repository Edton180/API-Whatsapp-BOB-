// API WhatsApp BOB - Desenvolvido por Edton Braz.

package calls

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/webrtc/v4"

	"github.com/edton180/api-whatsapp-bob/internal/config"
)

// ICEServer é um servidor STUN ou TURN no formato que o navegador entende (RTCIceServer).
type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// ICEServers devolve os servidores para uma chamada. Os de TURN recebem um usuário e uma senha
// temporários (padrão TURN REST): o segredo nunca sai da BOB e a credencial vence em TURNTTL.
func ICEServers(cfg config.WebRTC, user string, now time.Time) []ICEServer {
	var stun, turn []string
	for _, u := range cfg.ICEServers {
		low := strings.ToLower(u)
		switch {
		case strings.HasPrefix(low, "turn:"), strings.HasPrefix(low, "turns:"):
			turn = append(turn, u)
		case strings.HasPrefix(low, "stun:"), strings.HasPrefix(low, "stuns:"):
			stun = append(stun, u)
		}
	}
	out := []ICEServer{}
	if len(stun) > 0 {
		out = append(out, ICEServer{URLs: stun})
	}
	if len(turn) > 0 && cfg.TURNSecret != "" {
		ttl := cfg.TURNTTL
		if ttl <= 0 {
			ttl = 600
		}
		name := fmt.Sprintf("%d:%s", now.Add(time.Duration(ttl)*time.Second).Unix(), sanitizeUser(user))
		mac := hmac.New(sha1.New, []byte(cfg.TURNSecret))
		mac.Write([]byte(name))
		out = append(out, ICEServer{URLs: turn, Username: name, Credential: base64.StdEncoding.EncodeToString(mac.Sum(nil))})
	}
	return out
}

func sanitizeUser(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == ':' || r < 32 {
			return '-'
		}
		return r
	}, s)
	if len(s) > 64 {
		s = s[:64]
	}
	if s == "" {
		s = "bob"
	}
	return s
}

func (s ICEServer) pion() webrtc.ICEServer {
	out := webrtc.ICEServer{URLs: s.URLs, Username: s.Username}
	if s.Credential != "" {
		out.Credential = s.Credential
		out.CredentialType = webrtc.ICECredentialTypePassword
	}
	return out
}

// Codecs da ponte. Áudio: G.722 (16 kHz, sem reamostrar) e PCMU de reserva. Vídeo: H.264, o
// único que o WhatsApp usa, então o vídeo passa sem ser recodificado.
const (
	ptG722 = 9
	ptPCMU = 0
	ptH264 = 102
)

var h264Fmtp = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"

func newMediaEngine() (*webrtc.MediaEngine, error) {
	m := &webrtc.MediaEngine{}
	for _, c := range []webrtc.RTPCodecParameters{
		{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeG722, ClockRate: 8000, Channels: 1}, PayloadType: ptG722},
		{RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypePCMU, ClockRate: 8000, Channels: 1}, PayloadType: ptPCMU},
	} {
		if err := m.RegisterCodec(c, webrtc.RTPCodecTypeAudio); err != nil {
			return nil, err
		}
	}
	fb := []webrtc.RTCPFeedback{{Type: "goog-remb"}, {Type: "ccm", Parameter: "fir"}, {Type: "nack"}, {Type: "nack", Parameter: "pli"}}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeH264, ClockRate: 90000, SDPFmtpLine: h264Fmtp, RTCPFeedback: fb},
		PayloadType:        ptH264,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return nil, err
	}
	return m, nil
}

// newAPI monta a API do Pion com a faixa de portas, a porta única (UDP mux) e o IP público.
func newAPI(cfg config.WebRTC) (*webrtc.API, func() error, error) {
	m, err := newMediaEngine()
	if err != nil {
		return nil, nil, err
	}
	se := webrtc.SettingEngine{}
	closer := func() error { return nil }
	switch {
	case cfg.UDPMux > 0:
		conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: cfg.UDPMux})
		if err != nil {
			return nil, nil, fmt.Errorf("porta UDP %d das chamadas: %w", cfg.UDPMux, err)
		}
		mux := ice.NewUDPMuxDefault(ice.UDPMuxParams{UDPConn: conn})
		se.SetICEUDPMux(mux)
		closer = mux.Close
	case cfg.PortMin > 0 && cfg.PortMax >= cfg.PortMin:
		if err := se.SetEphemeralUDPPortRange(uint16(cfg.PortMin), uint16(cfg.PortMax)); err != nil {
			return nil, nil, fmt.Errorf("faixa de portas UDP das chamadas: %w", err)
		}
	}
	if cfg.PublicIP != "" {
		if err := se.SetICEAddressRewriteRules(webrtc.ICEAddressRewriteRule{
			External:        []string{cfg.PublicIP},
			AsCandidateType: webrtc.ICECandidateTypeHost,
			Mode:            webrtc.ICEAddressRewriteReplace,
		}); err != nil {
			return nil, nil, fmt.Errorf("WEBRTC_PUBLIC_IP: %w", err)
		}
	}
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled) // servidor: candidatos com IP, sem mDNS
	se.SetNetworkTypes([]webrtc.NetworkType{webrtc.NetworkTypeUDP4, webrtc.NetworkTypeUDP6})
	// NACK, relatórios RTCP e controle de banda padrão do Pion: a voz aguenta perda de pacotes.
	reg := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, reg); err != nil {
		return nil, nil, err
	}
	return webrtc.NewAPI(webrtc.WithMediaEngine(m), webrtc.WithSettingEngine(se), webrtc.WithInterceptorRegistry(reg)), closer, nil
}
