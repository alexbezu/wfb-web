package main

import (
	"log"
	"net/http"
	"os"

	"github.com/OpenIPC/wfb-web/internal/app"
	"github.com/OpenIPC/wfb-web/internal/frontend"
)

func main() {
	addr := env("WFB_WEB_ADDR", ":8080")
	cfgPath := env("WFB_WEB_CFG", "/etc/wifibroadcast.cfg")
	defaultPath := env("WFB_WEB_DEFAULT", "/etc/default/wifibroadcast")
	masterPath := env("WFB_WEB_MASTER_CFG", "")
	defaultProfile := env("WFB_WEB_PROFILE", "auto")
	statsAddr := env("WFB_WEB_STATS_ADDR", "127.0.0.1:8103")

	server := app.NewServer(cfgPath, defaultPath, masterPath, defaultProfile, statsAddr)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	frontend.RegisterRoutes(mux)

	log.Printf("wfb-web listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
