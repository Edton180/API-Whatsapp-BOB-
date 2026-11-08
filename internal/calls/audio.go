// API WhatsApp BOB - Desenvolvido por Edton Braz.

package calls

import (
	"strings"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/purpshell/meowcaller"
)

// Áudio entre as duas pontas: o motor do WhatsApp trabalha com PCM de 16 kHz mono em quadros
// de 60 ms (float32 de -1 a 1); o navegador recebe e envia pacotes de 20 ms em G.722 ou PCMU.

const (
	rate        = meowcaller.SampleRate // 16000
	packetSamp  = rate / 50             // 20 ms
	maxBuffered = rate * 3 / 10         // 300 ms: acima disso o atraso cresce, então o mais antigo sai
)

func toFloat(pcm []int16) []float32 {
	out := make([]float32, len(pcm))
	for i, v := range pcm {
		out[i] = float32(v) / 32768
	}
	return out
}

func toInt16(f []float32) []int16 {
	out := make([]int16, len(f))
	for i, v := range f {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		out[i] = int16(v * 32767)
	}
	return out
}

// audioCodec codifica e decodifica um dos formatos aceitos pelo navegador.
type audioCodec interface {
	encode(pcm []int16) []byte
	decode(b []byte) []int16
}

type g722Codec struct {
	enc *G722Encoder
	dec *G722Decoder
}

func (c *g722Codec) encode(pcm []int16) []byte { return c.enc.Encode(pcm) }
func (c *g722Codec) decode(b []byte) []int16   { return c.dec.Decode(b) }

// pcmuCodec é o G.711 µ-law de 8 kHz, a reserva para navegadores sem G.722.
type pcmuCodec struct{ last int16 }

func (c *pcmuCodec) encode(pcm []int16) []byte {
	out := make([]byte, 0, len(pcm)/2)
	for i := 0; i+1 < len(pcm); i += 2 {
		out = append(out, ulawEncode(int16((int(pcm[i])+int(pcm[i+1]))/2)))
	}
	return out
}

func (c *pcmuCodec) decode(b []byte) []int16 {
	out := make([]int16, 0, len(b)*2)
	for _, v := range b {
		s := ulawDecode(v)
		out = append(out, int16((int(c.last)+int(s))/2), s)
		c.last = s
	}
	return out
}

func newAudioCodec(mime string) audioCodec {
	if strings.EqualFold(mime, webrtc.MimeTypePCMU) {
		return &pcmuCodec{}
	}
	return &g722Codec{enc: NewG722Encoder(), dec: NewG722Decoder()}
}

const ulawBias = 0x84

func ulawEncode(s int16) byte {
	v := int(s)
	sign := 0
	if v < 0 {
		v, sign = -v, 0x80
	}
	if v > 32635 {
		v = 32635
	}
	v += ulawBias
	exp := 7
	for mask := 0x4000; exp > 0 && v&mask == 0; mask >>= 1 {
		exp--
	}
	mant := (v >> (exp + 3)) & 0x0F
	return ^byte(sign | exp<<4 | mant)
}

func ulawDecode(b byte) int16 {
	b = ^b
	exp := int(b>>4) & 0x07
	v := ((int(b&0x0F) << 3) + ulawBias) << exp
	v -= ulawBias
	if b&0x80 != 0 {
		v = -v
	}
	return int16(v)
}

// liveSource leva ao WhatsApp o áudio que chega do navegador. O motor pede um quadro a cada
// 60 ms; se ainda não chegou áudio suficiente, o resto do quadro vai em silêncio (nunca trava).
type liveSource struct {
	mu     sync.Mutex
	buf    []float32
	muted  bool
	closed bool
}

func (l *liveSource) push(samples []float32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.buf = append(l.buf, samples...)
	if over := len(l.buf) - maxBuffered; over > 0 {
		l.buf = l.buf[over:]
	}
}

func (l *liveSource) setMuted(m bool) {
	l.mu.Lock()
	l.muted = m
	l.buf = l.buf[:0]
	l.mu.Unlock()
}

func (l *liveSource) ReadFrame() ([]float32, error) {
	frame := make([]float32, meowcaller.FrameSamples)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.muted || l.closed {
		return frame, nil
	}
	n := copy(frame, l.buf)
	l.buf = l.buf[n:]
	return frame, nil
}

func (l *liveSource) Close() error {
	l.mu.Lock()
	l.closed = true
	l.buf = nil
	l.mu.Unlock()
	return nil
}

// sampleWriter é a parte da track local do Pion que a ponte usa (permite testar sem rede).
type sampleWriter interface {
	WriteSample(media.Sample) error
}

// browserSink leva ao navegador o áudio que chega do WhatsApp, em pacotes de 20 ms.
type browserSink struct {
	mu      sync.Mutex
	track   sampleWriter
	codec   audioCodec
	pending []int16
	closed  bool
}

func (b *browserSink) WriteFrame(frame []float32) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || b.track == nil {
		return nil
	}
	b.pending = append(b.pending, toInt16(frame)...)
	for len(b.pending) >= packetSamp {
		pkt := b.codec.encode(b.pending[:packetSamp])
		b.pending = b.pending[packetSamp:]
		if err := b.track.WriteSample(media.Sample{Data: pkt, Duration: 20 * time.Millisecond}); err != nil {
			return nil // navegador saiu: a chamada é encerrada pelo estado da conexão
		}
	}
	return nil
}

// setTrack troca o navegador que recebe o áudio (atender ou transferir). nil pausa o envio.
func (b *browserSink) setTrack(w sampleWriter, codec audioCodec) {
	b.mu.Lock()
	b.track = w
	b.codec = codec
	b.pending = b.pending[:0]
	b.mu.Unlock()
}

func (b *browserSink) Close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	return nil
}
