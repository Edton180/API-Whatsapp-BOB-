// API WhatsApp BOB - Desenvolvido por Edton Braz.

package calls

import (
	"math"
	"testing"
)

func tone(freq float64, n int, amp float64) []int16 {
	out := make([]int16, n)
	for i := range out {
		out[i] = int16(amp * math.Sin(2*math.Pi*freq*float64(i)/16000))
	}
	return out
}

// snr compara a saída com a entrada no atraso que melhor alinha as duas (o QMF atrasa ~22 amostras).
func snr(in, out []int16) (best float64, delay int) {
	best = math.Inf(-1)
	for dl := 0; dl < 64; dl++ {
		var sig, noise float64
		for i := 2000; i+dl < len(out) && i < len(in); i++ {
			s := float64(in[i])
			e := float64(out[i+dl]) - s
			sig += s * s
			noise += e * e
		}
		if noise == 0 {
			return math.Inf(1), dl
		}
		if v := 10 * math.Log10(sig/noise); v > best {
			best, delay = v, dl
		}
	}
	return best, delay
}

func TestG722RoundTrip(t *testing.T) {
	for _, f := range []float64{300, 1000, 2500, 5000, 6500} {
		in := tone(f, 16000, 8000)
		enc, dec := NewG722Encoder(), NewG722Decoder()
		var out []int16
		for i := 0; i < len(in); i += 320 { // pacotes de 20 ms, como no RTP
			out = append(out, dec.Decode(enc.Encode(in[i:i+320]))...)
		}
		if len(out) != len(in) {
			t.Fatalf("%v Hz: %d amostras na saída, esperado %d", f, len(out), len(in))
		}
		s, delay := snr(in, out)
		t.Logf("%v Hz: SNR %.1f dB, atraso %d amostras", f, s, delay)
		if s < 20 {
			t.Fatalf("%v Hz: SNR %.1f dB abaixo de 20 dB", f, s)
		}
	}
}

// Uma mistura de tons nas duas bandas só se alinha no atraso real do filtro (22 amostras).
func TestG722MixedBands(t *testing.T) {
	a, b, c := tone(440, 16000, 4000), tone(1730, 16000, 3000), tone(5210, 16000, 2000)
	in := make([]int16, len(a))
	for i := range in {
		in[i] = a[i] + b[i] + c[i]
	}
	enc, dec := NewG722Encoder(), NewG722Decoder()
	out := dec.Decode(enc.Encode(in))
	s, delay := snr(in, out)
	if delay != 22 || s < 20 {
		t.Fatalf("SNR %.1f dB com atraso %d; esperado atraso 22 e SNR acima de 20 dB", s, delay)
	}
}

func TestG722Silence(t *testing.T) {
	enc, dec := NewG722Encoder(), NewG722Decoder()
	out := dec.Decode(enc.Encode(make([]int16, 3200)))
	for i, v := range out {
		if v > 64 || v < -64 {
			t.Fatalf("silêncio virou ruído na amostra %d: %d", i, v)
		}
	}
}

func TestG722PacketSize(t *testing.T) {
	if n := len(NewG722Encoder().Encode(make([]int16, 320))); n != 160 {
		t.Fatalf("20 ms deveriam virar 160 bytes, viraram %d", n)
	}
}
