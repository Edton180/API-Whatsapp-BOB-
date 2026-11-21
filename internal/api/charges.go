// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

// Cobranças: a BOB monta a cobrança (Pix copia e cola e QR Code, link ou boleto), entrega ao
// cliente pelo WhatsApp, guarda a situação e avisa por evento. O Pix pode ser gerado pela
// própria BOB, com a chave da instância, ou vir pronto de um gateway de pagamento.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/edton180/api-whatsapp-bob/internal/engine"
	"github.com/edton180/api-whatsapp-bob/internal/pix"
	"github.com/edton180/api-whatsapp-bob/internal/store"
)

func (s *Server) registerCharges(r *gin.Engine, inst gin.HandlerFunc) {
	g := r.Group("/charge", inst)
	g.GET("/settings", s.chargeSettingsGet)
	g.PUT("/settings", s.chargeSettingsPut)
	g.POST("/create", s.chargeCreate)
	g.GET("/list", s.chargeList)
	g.GET("/:id", s.chargeGet)
	g.GET("/:id/qrcode", s.chargeQRCode)
	g.POST("/:id/send", s.chargeSend)
	g.POST("/:id/pay", s.chargePay)
	g.POST("/:id/cancel", s.chargeCancel)
}

// chargeJSON é a cobrança como a API devolve e como vai nos eventos.
func chargeJSON(c *store.Charge) gin.H {
	var meta any
	if c.Metadata != "" {
		_ = json.Unmarshal([]byte(c.Metadata), &meta)
	}
	return gin.H{
		"id": c.ID, "instanceId": c.InstanceID, "number": c.Number, "remoteJid": c.RemoteJid,
		"customerName": c.CustomerName, "description": c.Description,
		"amount": float64(c.AmountCents) / 100, "amountCents": c.AmountCents, "currency": c.Currency,
		"dueDate": c.DueDate, "status": c.Status, "overdue": c.Overdue(time.Now()),
		"pixCode": c.PixCode, "paymentUrl": c.PaymentURL, "boletoLine": c.BoletoLine,
		"reference": c.Reference, "metadata": meta,
		"sentCount": c.SentCount, "sentAt": c.SentAt, "paidAt": c.PaidAt, "cancelledAt": c.CancelledAt,
		"createdAt": c.CreatedAt, "updatedAt": c.UpdatedAt,
	}
}

func (s *Server) chargeSettingsGet(c *gin.Context) {
	ok(c, s.Repo.ChargeSettingsOf(current(c).Id))
}

func (s *Server) chargeSettingsPut(c *gin.Context) {
	var req store.ChargeSettings
	if !bind(c, &req) {
		return
	}
	req.InstanceID = current(c).Id
	req.PixKey, req.PixName, req.PixCity = strings.TrimSpace(req.PixKey), strings.TrimSpace(req.PixName), strings.TrimSpace(req.PixCity)
	req.PixKeyType = strings.ToLower(strings.TrimSpace(req.PixKeyType))
	if req.PixKey != "" {
		// confere a chave montando um BR Code de teste
		if _, err := pix.Code(pix.Params{Key: req.PixKey, KeyType: req.PixKeyType, Name: req.PixName, City: req.PixCity, AmountCents: 100}); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
	}
	if err := s.Repo.SaveChargeSettings(&req); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	ok(c, s.Repo.ChargeSettingsOf(req.InstanceID))
}

type chargeRequest struct {
	Number       string   `json:"number"`
	CustomerName string   `json:"customerName"`
	Description  string   `json:"description"`
	Amount       *float64 `json:"amount"`      // em reais: 49.9
	AmountCents  *int64   `json:"amountCents"` // ou em centavos: 4990
	Currency     string   `json:"currency"`
	DueDate      string   `json:"dueDate"`
	PixCode      string   `json:"pixCode"`    // Pix copia e cola pronto (de um gateway); vazio = gera com a chave da instância
	PaymentURL   string   `json:"paymentUrl"` // link de pagamento (cartão, checkout)
	BoletoLine   string   `json:"boletoLine"`
	Reference    string   `json:"reference"`
	Metadata     any      `json:"metadata"`
	Send         *bool    `json:"send"`    // envia ao cliente agora (padrão: true)
	Message      string   `json:"message"` // texto no lugar do padrão
	Delay        int      `json:"delay"`
}

func (s *Server) chargeCreate(c *gin.Context) {
	var req chargeRequest
	if !bind(c, &req) {
		return
	}
	inst := current(c)
	send := req.Send == nil || *req.Send
	req.Reference = strings.TrimSpace(req.Reference)

	// Mesma referência: devolve a cobrança que já existe (repetir a chamada não cobra duas vezes).
	if req.Reference != "" {
		if old, err := s.Repo.ChargeByRef(inst.Id, req.Reference); err == nil && old.Reference == req.Reference {
			s.respondCharge(c, old, send && old.Status == store.ChargePending, req.Message, req.Delay, false)
			return
		}
	}

	cents, err := chargeAmount(req)
	if err != nil {
		fail(c, http.StatusBadRequest, err)
		return
	}
	number := strings.TrimSpace(req.Number)
	if number == "" {
		fail(c, http.StatusBadRequest, errors.New("informe o number do cliente"))
		return
	}
	due := strings.TrimSpace(req.DueDate)
	if due != "" {
		if len(due) > 10 {
			due = due[:10]
		}
		if _, err := time.Parse("2006-01-02", due); err != nil {
			fail(c, http.StatusBadRequest, errors.New("dueDate deve estar no formato AAAA-MM-DD"))
			return
		}
	}
	link := strings.TrimSpace(req.PaymentURL)
	if link != "" {
		if u, err := url.Parse(link); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			fail(c, http.StatusBadRequest, errors.New("paymentUrl inválido"))
			return
		}
	}
	ch := &store.Charge{
		InstanceID: inst.Id, Number: number, CustomerName: cut(req.CustomerName, 160), Description: cut(req.Description, 500),
		AmountCents: cents, Currency: strings.ToUpper(strings.TrimSpace(req.Currency)), DueDate: due, Status: store.ChargePending,
		PixCode: strings.TrimSpace(req.PixCode), PaymentURL: link, BoletoLine: cut(req.BoletoLine, 120), Reference: cut(req.Reference, 128),
	}
	if ch.Currency == "" {
		ch.Currency = "BRL"
	}
	if req.Metadata != nil {
		if b, err := json.Marshal(req.Metadata); err == nil && len(b) <= 8000 {
			ch.Metadata = string(b)
		}
	}
	if ch.PixCode != "" && !pix.Valid(ch.PixCode) {
		fail(c, http.StatusBadRequest, errors.New("pixCode não é um Pix copia e cola válido"))
		return
	}
	// Sem Pix pronto, sem link e sem boleto: a BOB gera o Pix com a chave da instância.
	if ch.PixCode == "" && ch.PaymentURL == "" && ch.BoletoLine == "" {
		set := s.Repo.ChargeSettingsOf(inst.Id)
		if set.PixKey == "" {
			fail(c, http.StatusBadRequest, errors.New("cadastre a chave Pix da instância em PUT /charge/settings, ou envie pixCode, paymentUrl ou boletoLine"))
			return
		}
		if ch.Currency != "BRL" {
			fail(c, http.StatusBadRequest, errors.New("o Pix só aceita cobranças em BRL"))
			return
		}
		ch.ID = store.NewID()
		code, err := pix.Code(pix.Params{Key: set.PixKey, KeyType: set.PixKeyType, Name: set.PixName, City: set.PixCity, AmountCents: cents, TxID: "BOB" + strings.ReplaceAll(ch.ID, "-", "")[:20]})
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ch.PixCode = code
	}
	if send {
		// confere a conexão antes de gravar, para não deixar uma cobrança que não saiu
		if _, err := s.session(c).Client(); err != nil {
			failEngine(c, err)
			return
		}
	}
	if err := s.Repo.CreateCharge(ch); err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	s.session(c).Emit("ChargeCreated", chargeJSON(ch))
	s.respondCharge(c, ch, send, req.Message, req.Delay, true)
}

// respondCharge envia a cobrança (quando pedido) e devolve o resultado.
func (s *Server) respondCharge(c *gin.Context, ch *store.Charge, send bool, message string, delay int, created bool) {
	out := chargeJSON(ch)
	out["created"] = created
	out["sent"] = false
	if send {
		if err := s.deliverCharge(c.Request.Context(), s.session(c), ch, message, delay); err != nil {
			out["sendError"] = err.Error()
		} else {
			out = chargeJSON(ch)
			out["created"], out["sent"] = created, true
		}
	}
	ok(c, out)
}

func chargeAmount(req chargeRequest) (int64, error) {
	var cents int64
	switch {
	case req.AmountCents != nil:
		cents = *req.AmountCents
	case req.Amount != nil:
		cents = int64(math.Round(*req.Amount * 100))
	default:
		return 0, errors.New("informe o amount (valor da cobrança)")
	}
	if cents <= 0 {
		return 0, errors.New("o valor da cobrança deve ser maior que zero")
	}
	if cents > 99_999_999_99 {
		return 0, errors.New("valor da cobrança alto demais")
	}
	return cents, nil
}

// deliverCharge manda a cobrança ao cliente: o texto (com o QR Code do Pix, quando houver) e,
// em seguida, o Pix copia e cola sozinho numa mensagem, para ficar fácil de copiar.
func (s *Server) deliverCharge(ctx context.Context, sess *engine.Session, ch *store.Charge, message string, delay int) error {
	to, err := sess.ResolveJID(ctx, ch.Number)
	if err != nil {
		return err
	}
	text := strings.TrimSpace(message)
	if text == "" {
		text = ChargeText(ch, s.Repo.ChargeSettingsOf(ch.InstanceID).Message)
	}
	opt := engine.SendOptions{Delay: delay}
	sentImage := false
	if ch.PixCode != "" {
		if png, err := qrcode.Encode(ch.PixCode, qrcode.Medium, 512); err == nil {
			if msg, err := sess.Media(ctx, engine.MediaInput{Data: png, Type: "image", Mime: "image/png", Caption: text}); err == nil {
				if _, err = sess.Send(ctx, to, msg, opt); err != nil {
					return err
				}
				sentImage = true
			}
		}
	}
	if !sentImage {
		if _, err := sess.Send(ctx, to, engine.Text(text), opt); err != nil {
			return err
		}
	}
	if ch.PixCode != "" {
		if _, err := sess.Send(ctx, to, engine.Text(ch.PixCode), engine.SendOptions{}); err != nil {
			return err
		}
	}
	if err := s.Repo.MarkChargeSent(ch, to.String(), time.Now()); err != nil {
		return err
	}
	sess.Emit("ChargeSent", chargeJSON(ch))
	return nil
}

// Money escreve o valor no formato brasileiro: 1234567 -> "R$ 12.345,67".
func Money(cents int64, currency string) string {
	whole := strconv.FormatInt(cents/100, 10)
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "." + whole[i:]
	}
	symbol := "R$"
	if currency != "" && currency != "BRL" {
		symbol = currency
	}
	return fmt.Sprintf("%s %s,%02d", symbol, whole, cents%100)
}

func brDate(iso string) string {
	if t, err := time.Parse("2006-01-02", iso); err == nil {
		return t.Format("02/01/2006")
	}
	return iso
}

// fill troca {nome}, {descricao}, {valor} e {vencimento} no texto personalizado.
func fill(tpl string, ch *store.Charge) string {
	return strings.NewReplacer(
		"{nome}", ch.CustomerName, "{descricao}", ch.Description,
		"{valor}", Money(ch.AmountCents, ch.Currency), "{vencimento}", brDate(ch.DueDate),
	).Replace(tpl)
}

// ChargeText monta o texto da cobrança. custom é o texto da instância (vazio = padrão).
func ChargeText(ch *store.Charge, custom string) string {
	var b strings.Builder
	if custom = strings.TrimSpace(custom); custom != "" {
		b.WriteString(fill(custom, ch))
		b.WriteString("\n")
	} else {
		if ch.CustomerName != "" {
			fmt.Fprintf(&b, "Olá, %s!\n", ch.CustomerName)
		}
		b.WriteString("*Cobrança*")
		if ch.Description != "" {
			b.WriteString(": " + ch.Description)
		}
		b.WriteString("\n")
		fmt.Fprintf(&b, "Valor: *%s*\n", Money(ch.AmountCents, ch.Currency))
		if ch.DueDate != "" {
			fmt.Fprintf(&b, "Vencimento: %s\n", brDate(ch.DueDate))
		}
	}
	if ch.PixCode != "" {
		b.WriteString("\nPague com Pix: aponte a câmera do app do banco para o QR Code ou copie o código da próxima mensagem (Pix copia e cola).\n")
	}
	if ch.PaymentURL != "" {
		b.WriteString("\nLink de pagamento: " + ch.PaymentURL + "\n")
	}
	if ch.BoletoLine != "" {
		b.WriteString("\nBoleto (linha digitável): " + ch.BoletoLine + "\n")
	}
	return strings.TrimSpace(b.String())
}

// ReceiptText monta o aviso de pagamento confirmado.
func ReceiptText(ch *store.Charge, custom string) string {
	if custom = strings.TrimSpace(custom); custom != "" {
		return fill(custom, ch)
	}
	text := "✅ *Pagamento confirmado!*\nRecebemos " + Money(ch.AmountCents, ch.Currency)
	if ch.Description != "" {
		text += " referente a " + ch.Description
	}
	return text + ". Obrigado!"
}

func (s *Server) chargeOf(c *gin.Context) *store.Charge {
	ch, err := s.Repo.ChargeByRef(current(c).Id, c.Param("id"))
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, store.ErrChargeNotFound) {
			code = http.StatusNotFound
		}
		fail(c, code, err)
		return nil
	}
	return ch
}

func (s *Server) chargeGet(c *gin.Context) {
	if ch := s.chargeOf(c); ch != nil {
		ok(c, chargeJSON(ch))
	}
}

func (s *Server) chargeList(c *gin.Context) {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	status := strings.ToLower(c.Query("status"))
	switch status {
	case "", store.ChargePending, store.ChargePaid, store.ChargeCancelled, "overdue":
	default:
		fail(c, http.StatusBadRequest, errors.New("status deve ser pending, paid, cancelled ou overdue"))
		return
	}
	list, total, err := s.Repo.Charges(current(c).Id, store.ChargeQuery{Status: status, Number: c.Query("number"), Limit: limit, Offset: offset})
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	out := make([]gin.H, 0, len(list))
	for i := range list {
		out = append(out, chargeJSON(&list[i]))
	}
	ok(c, gin.H{"total": total, "charges": out})
}

// chargeQRCode devolve a imagem (PNG) do QR Code do Pix da cobrança.
func (s *Server) chargeQRCode(c *gin.Context) {
	ch := s.chargeOf(c)
	if ch == nil {
		return
	}
	if ch.PixCode == "" {
		fail(c, http.StatusBadRequest, errors.New("esta cobrança não tem Pix"))
		return
	}
	png, err := qrcode.Encode(ch.PixCode, qrcode.Medium, 512)
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	c.Data(http.StatusOK, "image/png", png)
}

// chargeSend reenvia a cobrança (lembrete).
func (s *Server) chargeSend(c *gin.Context) {
	ch := s.chargeOf(c)
	if ch == nil {
		return
	}
	var req struct {
		Message string `json:"message"`
		Delay   int    `json:"delay"`
	}
	_ = c.ShouldBindJSON(&req)
	if ch.Status != store.ChargePending {
		fail(c, http.StatusConflict, errors.New("só cobranças pendentes podem ser enviadas"))
		return
	}
	if err := s.deliverCharge(c.Request.Context(), s.session(c), ch, req.Message, req.Delay); err != nil {
		failEngine(c, err)
		return
	}
	ok(c, chargeJSON(ch))
}

// chargePay dá baixa na cobrança e, se pedido, avisa o cliente.
func (s *Server) chargePay(c *gin.Context) {
	ch := s.chargeOf(c)
	if ch == nil {
		return
	}
	var req struct {
		Notify  *bool  `json:"notify"` // envia o recibo ao cliente (padrão: true)
		Message string `json:"message"`
	}
	_ = c.ShouldBindJSON(&req)
	if ch.Status == store.ChargeCancelled {
		fail(c, http.StatusConflict, errors.New("esta cobrança foi cancelada"))
		return
	}
	changed, err := s.Repo.SetChargeStatus(ch, store.ChargePaid, time.Now())
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	out := chargeJSON(ch)
	out["changed"] = changed
	if !changed {
		// já estava paga: responde igual, sem avisar o cliente de novo
		if fresh, err := s.Repo.ChargeByRef(ch.InstanceID, ch.ID); err == nil {
			out = chargeJSON(fresh)
			out["changed"] = false
		}
		ok(c, out)
		return
	}
	sess := s.session(c)
	sess.Emit("ChargePaid", chargeJSON(ch))
	if req.Notify == nil || *req.Notify {
		text := strings.TrimSpace(req.Message)
		if text == "" {
			text = ReceiptText(ch, s.Repo.ChargeSettingsOf(ch.InstanceID).ReceiptMessage)
		}
		ctx := c.Request.Context()
		to, err := sess.ResolveJID(ctx, ch.Number)
		if err == nil {
			_, err = sess.Send(ctx, to, engine.Text(text), engine.SendOptions{})
		}
		out["notified"] = err == nil
		if err != nil {
			out["notifyError"] = err.Error()
		}
	}
	ok(c, out)
}

func (s *Server) chargeCancel(c *gin.Context) {
	ch := s.chargeOf(c)
	if ch == nil {
		return
	}
	if ch.Status == store.ChargePaid {
		fail(c, http.StatusConflict, errors.New("esta cobrança já foi paga"))
		return
	}
	changed, err := s.Repo.SetChargeStatus(ch, store.ChargeCancelled, time.Now())
	if err != nil {
		fail(c, http.StatusInternalServerError, err)
		return
	}
	if changed {
		s.session(c).Emit("ChargeCancelled", chargeJSON(ch))
	}
	ok(c, chargeJSON(ch))
}

func cut(s string, max int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > max {
		return string(r[:max])
	}
	return s
}
