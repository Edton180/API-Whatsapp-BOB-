// API WhatsApp BOB - Desenvolvido por Edton Braz.

// Package panel serve o painel web da BOB, a especificação OpenAPI e a página do Swagger.
package panel

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed static
var files embed.FS

//go:embed openapi.yaml
var openapi []byte

//go:embed openapi.json
var openapiJSON []byte

//go:embed swagger.html
var swagger []byte

// Register publica o painel em /painel/.
func Register(r *gin.Engine) {
	sub, _ := fs.Sub(files, "static")
	r.GET("/painel", func(c *gin.Context) { c.Redirect(http.StatusFound, "/painel/") })
	r.GET("/painel/*file", func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache")
		c.FileFromFS(c.Param("file"), http.FS(sub))
	})
	r.GET("/favicon.ico", func(c *gin.Context) { c.FileFromFS("icone.png", http.FS(sub)) })
}

// OpenAPI devolve a especificação da API.
func OpenAPI() []byte { return openapi }

// OpenAPIJSON devolve a especificação em JSON.
func OpenAPIJSON() []byte { return openapiJSON }

// SwaggerPage devolve a página que mostra a especificação.
func SwaggerPage() []byte { return swagger }
