package main

import (
	"log"

	"github.com/Edton180/API-Whatsapp-BOB-/pkg/config"
	"github.com/Edton180/API-Whatsapp-BOB-/pkg/core"
	"github.com/Edton180/API-Whatsapp-BOB-/pkg/middleware"
	"github.com/Edton180/API-Whatsapp-BOB-/pkg/routes"
	"github.com/gin-gonic/gin"
)

func main() {
	config.LoadConfig()
	core.InitBobCore()

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(middleware.AuthMiddleware())

	routes.RegisterAllEvolutionAndBobRoutes(r)

	log.Printf("🚀 API WhatsApp BOB ativa na porta %s (Evolution Go Engine + Evolution API Functions)", config.Env.ServerPort)
	if err := r.Run(":" + config.Env.ServerPort); err != nil {
		log.Fatalf("Erro fatal: %v", err)
	}
}
