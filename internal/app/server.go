package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/OpenIPC/wfb-web/internal/config"
	"github.com/OpenIPC/wfb-web/internal/keystore"
	"github.com/OpenIPC/wfb-web/internal/profile"
	"github.com/OpenIPC/wfb-web/internal/radio"
	"github.com/OpenIPC/wfb-web/internal/service"
	"github.com/OpenIPC/wfb-web/internal/stats"
)

type Server struct {
	cfgPath     string
	defaultPath string
	masterPath  string
	mu          sync.RWMutex
	profile     profile.Selection
}

func NewServer(cfgPath, defaultPath, masterPath, defaultProfile string) *Server {
	selection := profile.Detect(defaultProfile)
	return &Server{cfgPath: cfgPath, defaultPath: defaultPath, masterPath: masterPath, profile: selection}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/profile", s.getProfile)
	mux.HandleFunc("PUT /api/profile", s.putProfile)
	mux.HandleFunc("GET /api/config", s.getConfig)
	mux.HandleFunc("GET /api/config/effective", s.getEffectiveConfig)
	mux.HandleFunc("PUT /api/config", s.putConfig)
	mux.HandleFunc("PUT /api/config/params", s.putConfigParams)
	mux.HandleFunc("GET /api/services", s.getServices)
	mux.HandleFunc("POST /api/services/", s.postService)
	mux.HandleFunc("GET /api/radio", s.getRadio)
	mux.HandleFunc("GET /api/key", s.getKey)
	mux.HandleFunc("PUT /api/key", s.putKey)
	mux.HandleFunc("GET /api/stats/stream", s.streamStats)
}

func (s *Server) getProfile(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	selection := s.profile
	s.mu.RUnlock()
	writeJSON(w, http.StatusOK, selection)
}

func (s *Server) putProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err)
		return
	}
	if !profile.Allowed(req.Profile) {
		writeError(w, errors.New("unsupported profile"))
		return
	}
	selection := profile.Manual(req.Profile)
	s.mu.Lock()
	s.profile = selection
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, selection)
}

func (s *Server) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.LoadWithMaster(s.masterPath, s.cfgPath, s.defaultPath)
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
	if err := config.SaveDiff(s.masterPath, s.cfgPath, s.defaultPath, cfg); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) putConfigParams(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Updates []config.ParameterUpdate `json:"updates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, err)
		return
	}
	if err := config.SaveParameters(s.masterPath, s.cfgPath, s.defaultPath, req.Updates); err != nil {
		writeError(w, err)
		return
	}
	cfg, err := config.LoadEffective(s.masterPath, s.cfgPath, s.defaultPath)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) getServices(w http.ResponseWriter, r *http.Request) {
	states, err := service.Status("wifibroadcast@gs", "wifibroadcast@drone", "rtsp@h265", "rtsp@h264", "fpv-camera.service")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, states)
}

func (s *Server) getRadio(w http.ResponseWriter, r *http.Request) {
	cfg, _ := config.LoadWithMaster(s.masterPath, s.cfgPath, s.defaultPath)
	writeJSON(w, http.StatusOK, radio.Inspect(strings.Fields(cfg.Default.WFBNics)))
}

func (s *Server) getKey(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	profileName := s.profile.Profile
	s.mu.RUnlock()
	info, err := keystore.Read(profileName)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (s *Server) putKey(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	profileName := s.profile.Profile
	s.mu.RUnlock()
	info, err := keystore.Save(profileName, r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
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
	s.mu.RLock()
	selection := s.profile
	s.mu.RUnlock()
	addr := "127.0.0.1:8103"
	if selection.Profile == "drone" {
		addr = "127.0.0.1:8102"
	}
	stats.ProxySSE(w, r, addr)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}
