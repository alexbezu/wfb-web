package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/OpenIPC/wfb-web/internal/config"
	"github.com/OpenIPC/wfb-web/internal/radio"
	"github.com/OpenIPC/wfb-web/internal/service"
	"github.com/OpenIPC/wfb-web/internal/stats"
)

type Server struct {
	cfgPath     string
	defaultPath string
	masterPath  string
	statsAddr   string
}

func NewServer(cfgPath, defaultPath, masterPath, statsAddr string) *Server {
	return &Server{cfgPath: cfgPath, defaultPath: defaultPath, masterPath: masterPath, statsAddr: statsAddr}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("GET /api/config/effective", s.getEffectiveConfig)
	mux.HandleFunc("PUT /api/config", s.putConfig)
	mux.HandleFunc("GET /api/services", s.getServices)
	mux.HandleFunc("POST /api/services/", s.postService)
	mux.HandleFunc("GET /api/radio", s.getRadio)
	mux.HandleFunc("GET /api/stats/stream", s.streamStats)
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(s.cfgPath, s.defaultPath)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) getEffectiveConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadEffective(s.masterPath, s.cfgPath, s.defaultPath)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, err)
		return
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, err)
		return
	}
	if err := config.Save(s.cfgPath, s.defaultPath, cfg); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) getServices(w http.ResponseWriter, r *http.Request) {
	states, err := service.Status("wifibroadcast@gs", "rtsp@h265", "rtsp@h264")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, states)
}

func (s *Server) getRadio(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.Load(s.cfgPath, s.defaultPath)
	writeJSON(w, http.StatusOK, radio.Inspect(strings.Fields(cfg.Default.WFBNics)))
}

func (s *Server) postService(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/services/"), "/")
	if len(parts) != 2 {
		writeError(w, errors.New("expected /api/services/{unit}/{action}"))
		return
	}
	unit, ok := service.AllowedUnit(parts[0])
	if !ok {
		writeError(w, errors.New("unknown service unit"))
		return
	}
	if err := service.Run(unit, parts[1]); err != nil {
		writeError(w, err)
		return
	}
	states, err := service.Status(unit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, states[0])
}

func (s *Server) streamStats(w http.ResponseWriter, r *http.Request) {
	stats.ProxySSE(w, r, s.statsAddr)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}
