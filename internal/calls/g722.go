// API WhatsApp BOB - Desenvolvido por Edton Braz.

package calls

// Codec G.722 (ITU-T G.722, modo de 64 kbit/s): áudio de 16 kHz em 8 bits por par de amostras.
// É o codec de banda larga que os navegadores aceitam no WebRTC sem Opus, e tem a mesma taxa
// do MLOW usado nas chamadas do WhatsApp, então não precisa reamostrar.
//
// Implementação própria a partir da descrição da recomendação: filtro QMF de 24 coeficientes
// separa as bandas baixa (0 a 4 kHz, ADPCM de 6 bits) e alta (4 a 8 kHz, ADPCM de 2 bits).

// Coeficientes do filtro QMF (metade par e metade ímpar do filtro simétrico de 24 pontos).
var qmfEven = [12]int{3, -11, 12, 32, -210, 951, 3876, -805, 362, -156, 53, -11}
var qmfOdd = [12]int{-11, 53, -156, 362, -805, 3876, 951, -210, 32, 12, -11, 3}

// Banda baixa: níveis de decisão, códigos, fatores de escala e reconstrução.
var q6 = [32]int{0, 35, 72, 110, 150, 190, 233, 276, 323, 370, 422, 473, 530, 587, 650, 714,
	786, 858, 940, 1023, 1121, 1219, 1339, 1458, 1612, 1765, 1980, 2195, 2557, 2919, 0, 0}
var iln = [32]int{0, 63, 62, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19,
	18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 0}
var ilp = [32]int{0, 61, 60, 59, 58, 57, 56, 55, 54, 53, 52, 51, 50, 49, 48, 47,
	46, 45, 44, 43, 42, 41, 40, 39, 38, 37, 36, 35, 34, 33, 32, 0}
var wl = [8]int{-60, -30, 58, 172, 334, 538, 1198, 3042}
var rl42 = [16]int{0, 7, 6, 5, 4, 3, 2, 1, 7, 6, 5, 4, 3, 2, 1, 0}
var ilb = [32]int{2048, 2093, 2139, 2186, 2233, 2282, 2332, 2383, 2435, 2489, 2543, 2599, 2656, 2714, 2774, 2834,
	2896, 2960, 3025, 3091, 3158, 3228, 3298, 3371, 3444, 3520, 3597, 3676, 3756, 3838, 3922, 4008}
var qm4 = [16]int{0, -20456, -12896, -8968, -6288, -4240, -2584, -1200,
	20456, 12896, 8968, 6288, 4240, 2584, 1200, 0}
var qm6 = [64]int{-136, -136, -136, -136, -24808, -21904, -19008, -16704,
	-14984, -13512, -12280, -11192, -10232, -9360, -8576, -7856,
	-7192, -6576, -6000, -5456, -4944, -4464, -4008, -3576,
	-3168, -2776, -2400, -2032, -1688, -1360, -1040, -728,
	24808, 21904, 19008, 16704, 14984, 13512, 12280, 11192,
	10232, 9360, 8576, 7856, 7192, 6576, 6000, 5456,
	4944, 4464, 4008, 3576, 3168, 2776, 2400, 2032,
	1688, 1360, 1040, 728, 432, 136, -432, -136}

// Banda alta.
var qm2 = [4]int{-7408, -1616, 7408, 1616}
var ihn = [3]int{0, 1, 0}
var ihp = [3]int{0, 3, 2}
var wh = [3]int{0, -214, 798}
var rh2 = [4]int{2, 1, 2, 1}

func sat16(v int) int {
	if v > 32767 {
		return 32767
	}
	if v < -32768 {
		return -32768
	}
	return v
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// adpcmBand é o estado do preditor adaptativo de uma sub-banda.
type adpcmBand struct {
	s, sp, sz int
	r, a, ap  [3]int
	p         [3]int
	d, b, bp  [7]int
	nb, det   int
}

// update recebe a diferença quantizada d e atualiza o preditor (bloco 4 da recomendação).
func (b *adpcmBand) update(d int) {
	b.d[0] = d
	b.r[0] = sat16(b.s + d)
	b.p[0] = sat16(b.sz + d)

	// Coeficiente de segunda ordem dos polos.
	sg0, sg1, sg2 := b.p[0]>>15, b.p[1]>>15, b.p[2]>>15
	wd1 := sat16(b.a[1] << 2)
	wd2 := wd1
	if sg0 == sg1 {
		wd2 = -wd1
	}
	if wd2 > 32767 {
		wd2 = 32767
	}
	wd3 := -128
	if sg0 == sg2 {
		wd3 = 128
	}
	wd3 += wd2 >> 7
	wd3 += (b.a[2] * 32512) >> 15
	b.ap[2] = clamp(wd3, -12288, 12288)

	// Coeficiente de primeira ordem dos polos.
	wd1 = -192
	if sg0 == sg1 {
		wd1 = 192
	}
	wd2 = (b.a[1] * 32640) >> 15
	b.ap[1] = sat16(wd1 + wd2)
	wd3 = sat16(15360 - b.ap[2])
	b.ap[1] = clamp(b.ap[1], -wd3, wd3)

	// Coeficientes dos zeros.
	step := 128
	if d == 0 {
		step = 0
	}
	sgd := d >> 15
	for i := 1; i < 7; i++ {
		w := -step
		if b.d[i]>>15 == sgd {
			w = step
		}
		b.bp[i] = sat16(w + (b.b[i]*32640)>>15)
	}

	// Atrasos.
	for i := 6; i > 0; i-- {
		b.d[i] = b.d[i-1]
		b.b[i] = b.bp[i]
	}
	for i := 2; i > 0; i-- {
		b.r[i] = b.r[i-1]
		b.p[i] = b.p[i-1]
		b.a[i] = b.ap[i]
	}

	// Previsão: parte dos polos e parte dos zeros.
	wd1 = (b.a[1] * sat16(b.r[1]+b.r[1])) >> 15
	wd2 = (b.a[2] * sat16(b.r[2]+b.r[2])) >> 15
	b.sp = sat16(wd1 + wd2)
	b.sz = 0
	for i := 6; i > 0; i-- {
		b.sz += (b.b[i] * sat16(b.d[i]+b.d[i])) >> 15
	}
	b.sz = sat16(b.sz)
	b.s = sat16(b.sp + b.sz)
}

// scale calcula o novo fator de escala a partir do logaritmo nb.
func scale(nb, shift int) int {
	wd1 := (nb >> 6) & 31
	wd2 := shift - (nb >> 11)
	var wd3 int
	if wd2 < 0 {
		wd3 = ilb[wd1] << -wd2
	} else {
		wd3 = ilb[wd1] >> wd2
	}
	return wd3 << 2
}

func (b *adpcmBand) adaptLow(ril int) {
	nb := (b.nb*127)>>7 + wl[rl42[ril]]
	b.nb = clamp(nb, 0, 18432)
	b.det = scale(b.nb, 8)
}

func (b *adpcmBand) adaptHigh(ih int) {
	nb := (b.nb*127)>>7 + wh[rh2[ih]]
	b.nb = clamp(nb, 0, 22528)
	b.det = scale(b.nb, 10)
}

// G722Encoder converte PCM de 16 kHz (int16) em G.722. Cada par de amostras vira um byte.
type G722Encoder struct {
	x    [24]int
	low  adpcmBand
	high adpcmBand
}

// NewG722Encoder cria um codificador com o estado inicial da recomendação.
func NewG722Encoder() *G722Encoder {
	e := &G722Encoder{}
	e.low.det, e.high.det = 32, 8
	return e
}

// Encode codifica as amostras (quantidade par). Uma amostra que sobrar é ignorada.
func (e *G722Encoder) Encode(pcm []int16) []byte {
	out := make([]byte, 0, len(pcm)/2)
	for j := 0; j+1 < len(pcm); j += 2 {
		copy(e.x[:22], e.x[2:])
		e.x[22], e.x[23] = int(pcm[j]), int(pcm[j+1])
		sumOdd, sumEven := 0, 0
		for i := 0; i < 12; i++ {
			sumOdd += e.x[2*i] * qmfEven[i]
			sumEven += e.x[2*i+1] * qmfOdd[i]
		}
		xlow := (sumEven + sumOdd) >> 14
		xhigh := (sumEven - sumOdd) >> 14

		// Banda baixa: 6 bits.
		el := sat16(xlow - e.low.s)
		wd := el
		if el < 0 {
			wd = -(el + 1)
		}
		i := 1
		for ; i < 30; i++ {
			if wd < (q6[i]*e.low.det)>>12 {
				break
			}
		}
		ilow := ilp[i]
		if el < 0 {
			ilow = iln[i]
		}
		ril := ilow >> 2
		dlow := (e.low.det * qm4[ril]) >> 15
		e.low.adaptLow(ril)
		e.low.update(dlow)

		// Banda alta: 2 bits.
		eh := sat16(xhigh - e.high.s)
		wd = eh
		if eh < 0 {
			wd = -(eh + 1)
		}
		mih := 1
		if wd >= (564*e.high.det)>>12 {
			mih = 2
		}
		ihigh := ihp[mih]
		if eh < 0 {
			ihigh = ihn[mih]
		}
		dhigh := (e.high.det * qm2[ihigh]) >> 15
		e.high.adaptHigh(ihigh)
		e.high.update(dhigh)

		out = append(out, byte(ihigh<<6|ilow))
	}
	return out
}

// G722Decoder converte G.722 em PCM de 16 kHz. Cada byte vira duas amostras.
type G722Decoder struct {
	x    [24]int
	low  adpcmBand
	high adpcmBand
}

// NewG722Decoder cria um decodificador com o estado inicial da recomendação.
func NewG722Decoder() *G722Decoder {
	d := &G722Decoder{}
	d.low.det, d.high.det = 32, 8
	return d
}

// Decode decodifica os bytes recebidos.
func (d *G722Decoder) Decode(data []byte) []int16 {
	out := make([]int16, 0, len(data)*2)
	for _, code := range data {
		ilow := int(code) & 0x3F
		ihigh := (int(code) >> 6) & 0x03

		rlow := clamp(d.low.s+(d.low.det*qm6[ilow])>>15, -16384, 16383)
		ril := ilow >> 2
		dlow := (d.low.det * qm4[ril]) >> 15
		d.low.adaptLow(ril)
		d.low.update(dlow)

		dhigh := (d.high.det * qm2[ihigh]) >> 15
		rhigh := clamp(dhigh+d.high.s, -16384, 16383)
		d.high.adaptHigh(ihigh)
		d.high.update(dhigh)

		copy(d.x[:22], d.x[2:])
		d.x[22], d.x[23] = rlow+rhigh, rlow-rhigh
		xout1, xout2 := 0, 0
		for i := 0; i < 12; i++ {
			xout2 += d.x[2*i] * qmfEven[i]
			xout1 += d.x[2*i+1] * qmfOdd[i]
		}
		out = append(out, int16(sat16(xout1>>11)), int16(sat16(xout2>>11)))
	}
	return out
}
