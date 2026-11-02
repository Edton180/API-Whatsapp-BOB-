package core

import "log"

type BobCoreInfo struct {
	Application        string   `json:"application"`
	Version            string   `json:"version"`
	Engine             string   `json:"engine"`
	Compatibility      string   `json:"compatibility"`
	LicenseActive      bool     `json:"licenseActive"`
	OpenSource         bool     `json:"openSource"`
	Creator            string   `json:"creator"`
	OfficialRepository string   `json:"officialRepository"`
	BaseRepositories   []string `json:"baseRepositories"`
}

var Info = BobCoreInfo{
	Application:        "API WhatsApp BOB",
	Version:            "2.0.0-GO",
	Engine:             "Evolution Go (Golang + whatsmeow)",
	Compatibility:      "Full Evolution API v2 Parity",
	LicenseActive:      true,
	OpenSource:         true,
	Creator:            "Edton Kaik Braz Da Conceição (@Edton180)",
	OfficialRepository: "https://github.com/Edton180/API-Whatsapp-BOB-",
	BaseRepositories: []string{
		"https://github.com/evolution-foundation/evolution-go",
		"https://github.com/evolution-foundation/evolution-api",
	},
}

func InitBobCore() {
	Info.LicenseActive = true
	log.Printf("[BOB-CORE] %s (%s) inicializada sem bloqueios 503.", Info.Application, Info.Version)
	log.Printf("[BOB-CORE] Criador Oficial: %s", Info.Creator)
}
