// API WhatsApp BOB - Desenvolvido por Edton Braz.

package api

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/edton180/api-whatsapp-bob/internal/media"
	"github.com/edton180/api-whatsapp-bob/internal/version"
)

func (s *Server) home(c *gin.Context) {
	if s.Engine.Cfg.PanelEnabled && strings.Contains(c.GetHeader("Accept"), "text/html") {
		c.Redirect(http.StatusFound, "/painel/")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"name":       version.Name,
		"version":    version.Current(),
		"commit":     version.Commit(),
		"creator":    version.Creator,
		"repository": version.Repository,
		"engine":     version.Engine,
		"panel":      "/painel/",
		"docs":       "/swagger/index.html",
		"donate":     "/donate/pix",
	})
}

// license mantém as rotas antigas de licença respondendo "ativa": a BOB não exige licença.
func (s *Server) license(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "active",
		"mode":    "free",
		"message": "A API WhatsApp BOB é livre e não precisa de licença.",
	})
}

func (s *Server) serverInfo(c *gin.Context) {
	total, connected := s.Engine.Stats()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	ok(c, gin.H{
		"name":          version.Name,
		"version":       version.Current(),
		"commit":        version.Commit(),
		"engine":        version.Engine,
		"go":            runtime.Version(),
		"uptime":        time.Since(s.Engine.Started).Round(time.Second).String(),
		"startedAt":     s.Engine.Started.Format(time.RFC3339),
		"instances":     total,
		"connected":     connected,
		"websockets":    s.Engine.Events.Hub.Count(),
		"memoryMB":      fmt.Sprintf("%.1f", float64(mem.Alloc)/1024/1024),
		"ffmpeg":        media.HasFFmpeg(),
		"database":      map[bool]string{true: "PostgreSQL", false: "SQLite"}[s.Engine.Cfg.UsersDB != ""],
		"globalWebhook": s.Engine.Cfg.GlobalWebhook != "",
	})
}

var upgrader = websocket.Upgrader{
	ReadBufferSize: 1024, WriteBufferSize: 4096,
	CheckOrigin: func(*http.Request) bool { return true },
}

// websocket transmite os eventos em tempo real.
//
//	/ws?token=<token da instância>      eventos de uma instância
//	/ws?apikey=<chave global>[&instance=]  todas (ou uma) instâncias
func (s *Server) websocket(c *gin.Context) {
	key := c.Query("apikey")
	if key == "" {
		key = c.Query("token")
	}
	if key == "" {
		key = apiKey(c)
	}
	filter := ""
	if s.isGlobal(key) {
		if ref := c.Query("instance"); ref != "" {
			inst, err := s.Repo.ByIDOrName(ref)
			if err != nil {
				fail(c, http.StatusNotFound, err)
				return
			}
			filter = inst.Id
		}
	} else {
		inst, err := s.Repo.ByToken(key)
		if err != nil {
			fail(c, http.StatusUnauthorized, err)
			return
		}
		filter = inst.Id
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	ch := s.Engine.Events.Hub.Subscribe(filter)
	defer s.Engine.Events.Hub.Unsubscribe(ch)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-done:
			return
		case msg, open := <-ch:
			if !open {
				return
			}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if conn.WriteMessage(websocket.TextMessage, msg) != nil {
				return
			}
		case <-ping.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}

// ---------------------------------------------------------------- Pix de apoio ao projeto

const (
	pixKey  = "04646947285"
	pixName = "EDTON KAIK BRAZ"
	pixCity = "BRASIL"
)

// PixPayload monta o BR Code estático (padrão EMV do Banco Central), com valor livre.
func PixPayload() string {
	field := func(id, v string) string { return fmt.Sprintf("%s%02d%s", id, len(v), v) }
	account := field("00", "br.gov.bcb.pix") + field("01", pixKey)
	p := field("00", "01") + field("26", account) + field("52", "0000") + field("53", "986") +
		field("58", "BR") + field("59", pixName) + field("60", pixCity) + field("62", field("05", "***")) + "6304"
	return p + fmt.Sprintf("%04X", PixCRC16(p))
}

// PixCRC16 calcula o CRC16-CCITT (inicial 0xFFFF, polinômio 0x1021) usado no BR Code.
func PixCRC16(s string) uint16 {
	crc := uint16(0xFFFF)
	for i := 0; i < len(s); i++ {
		crc ^= uint16(s[i]) << 8
		for b := 0; b < 8; b++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// PixInfo devolve os dados de doação ao projeto.
func PixInfo() map[string]string {
	return map[string]string{
		"project":      version.Name,
		"creator":      version.Creator,
		"pix_key":      pixKey,
		"pix_key_type": "CPF",
		"copia_e_cola": PixPayload(),
		"qr_code":      version.Repository + "/blob/main/docs/imagens/pix-qrcode.png",
	}
}
