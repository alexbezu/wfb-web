package app

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/OpenIPC/wfb-web/internal/camera"
	"github.com/OpenIPC/wfb-web/internal/config"
	"github.com/OpenIPC/wfb-web/internal/keystore"
	"github.com/OpenIPC/wfb-web/internal/profile"
	"github.com/OpenIPC/wfb-web/internal/radio"
	"github.com/OpenIPC/wfb-web/internal/rtsp"
	"github.com/OpenIPC/wfb-web/internal/service"
	"github.com/OpenIPC/wfb-web/internal/stats"
)

type Server struct {
	cfgPath     string
	defaultPath string
	masterPath  string
	mu          sync.RWMutex
	profile     profile.Selection
	camera      *camera.Manager
	rtsp        *rtsp.Manager
}

func NewServer(cfgPath, defaultPath, masterPath, defaultProfile string) *Server {
	cfg, _ := config.LoadWithMaster(masterPath, cfgPath, defaultPath)
	selection := profile.Detect(cfg.Default.Profile, defaultProfile)
	return &Server{cfgPath: cfgPath, defaultPath: defaultPath, masterPath: masterPath, profile: selection, camera: camera.NewManager(), rtsp: rtsp.NewManager()}
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
	mux.HandleFunc("GET /api/camera", s.getCamera)
	mux.HandleFunc("POST /api/camera/", s.postCamera)
	mux.HandleFunc("GET /api/rtsp", s.getRTSP)
	mux.HandleFunc("POST /api/rtsp/", s.postRTSP)
	mux.HandleFunc("GET /api/radio", s.getRadio)
	mux.HandleFunc("GET /api/key", s.getKey)
	mux.HandleFunc("PUT /api/key", s.putKey)
	mux.HandleFunc("GET /api/stats/stream", s.streamStats)
}

func (s *Server) ReconcileRuntime() {
	cfg, err := config.LoadWithMaster(s.masterPath, s.cfgPath, s.defaultPath)
	if err != nil {
		log.Printf("runtime reconcile config load failed: %v", err)
		return
	}
	if !cfg.Default.AutoServices {
		return
	}
	if err := s.reconcileCamera(); err != nil {
		log.Printf("camera reconcile failed: %v", err)
	}
	if err := s.reconcileRTSP(); err != nil {
		log.Printf("rtsp reconcile failed: %v", err)
	}
}

func (s *Server) selectedProfile() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profile.Profile
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
	if err := config.SaveParameters(s.masterPath, s.cfgPath, s.defaultPath, []config.ParameterUpdate{
		{Section: "default", Key: "WFB_WEB_PROFILE", Value: req.Profile},
	}); err != nil {
		writeError(w, err)
		return
	}
	selection := profile.Saved(req.Profile)
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
	s.ReconcileRuntime()
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
	s.ReconcileRuntime()
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
	states = append(states, rtspServiceState(s.rtspStatus()))
	states = append(states, cameraServiceState(s.cameraStatus()))
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
	if !s.serviceAllowedForProfile(parts[0]) {
		writeError(w, errors.New("service is not editable for selected profile"))
		return
	}
	if parts[0] == "wfb-web-rtsp" {
		s.handleRTSPAction(w, parts[1])
		return
	}
	if parts[0] == "wfb-web-camera" {
		s.handleCameraAction(w, parts[1])
		return
	}
	unit, ok := service.AllowedUnit(parts[0])
	if !ok {
		writeError(w, errors.New("unknown service unit"))
		return
	}
	if isSystemdRTSPUnit(unit) && (parts[1] == "start" || parts[1] == "restart") {
		if err := s.rtsp.Stop(); err != nil {
			writeError(w, err)
			return
		}
	}
	if isSystemdCameraUnit(unit) && (parts[1] == "start" || parts[1] == "restart") {
		if err := s.camera.Stop(); err != nil {
			writeError(w, err)
			return
		}
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

func (s *Server) serviceAllowedForProfile(key string) bool {
	s.mu.RLock()
	profileName := s.profile.Profile
	s.mu.RUnlock()
	switch profileName {
	case "drone":
		switch key {
		case "wifibroadcast-gs", "rtsp-h265", "rtsp-h264", "wfb-web-rtsp":
			return false
		}
	case "gs":
		switch key {
		case "wifibroadcast-drone", "fpv-camera", "wfb-web-camera":
			return false
		}
	}
	return true
}

func (s *Server) getCamera(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cameraStatus())
}

func (s *Server) postCamera(w http.ResponseWriter, r *http.Request) {
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/camera/"), "/")
	s.handleCameraAction(w, action)
}

func (s *Server) handleCameraAction(w http.ResponseWriter, action string) {
	opts := s.cameraOptions()
	var err error
	switch action {
	case "start":
		err = s.camera.Start(opts)
	case "stop":
		err = s.camera.Stop()
	case "restart":
		err = s.camera.Restart(opts)
	default:
		err = errors.New("unsupported camera action")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.cameraStatus())
}

func (s *Server) cameraStatus() camera.State {
	return s.camera.Status(s.cameraOptions())
}

func (s *Server) cameraOptions() camera.Options {
	cfg, _ := config.LoadWithMaster(s.masterPath, s.cfgPath, s.defaultPath)
	return camera.Options{
		Enabled:   cfg.Default.CameraEnabled,
		Source:    cfg.Default.CameraSource,
		Device:    cfg.Default.CameraDevice,
		RTSPURL:   cfg.Default.CameraRTSPURL,
		Codec:     cfg.Default.CameraCodec,
		Host:      cfg.Default.CameraHost,
		Port:      cfg.Default.CameraPort,
		Width:     cfg.Default.CameraWidth,
		Height:    cfg.Default.CameraHeight,
		Framerate: cfg.Default.CameraFramerate,
		Bitrate:   cfg.Default.CameraBitrate,
		MTU:       cfg.Default.CameraMTU,
		Pattern:   cfg.Default.CameraTestPattern,
	}
}

func (s *Server) getRTSP(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.rtspStatus())
}

func (s *Server) postRTSP(w http.ResponseWriter, r *http.Request) {
	action := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/rtsp/"), "/")
	s.handleRTSPAction(w, action)
}

func (s *Server) handleRTSPAction(w http.ResponseWriter, action string) {
	opts := s.rtspOptions()
	var err error
	switch action {
	case "start":
		err = s.rtsp.Start(opts)
	case "stop":
		err = s.rtsp.Stop()
	case "restart":
		err = s.rtsp.Restart(opts)
	default:
		err = errors.New("unsupported rtsp action")
	}
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.rtspStatus())
}

func (s *Server) rtspStatus() rtsp.State {
	return s.rtsp.Status(s.rtspOptions())
}

func (s *Server) rtspOptions() rtsp.Options {
	cfg, _ := config.LoadWithMaster(s.masterPath, s.cfgPath, s.defaultPath)
	return rtsp.Options{
		Codec:   cfg.Default.RTSPCodec,
		MTU:     cfg.Default.RTPMTU,
		Port:    cfg.Default.RTSPPort,
		URI:     cfg.Default.RTSPURI,
		Latency: cfg.Default.RTPJitter,
		RTPPort: 5600,
	}
}

func (s *Server) reconcileRTSP() error {
	if s.selectedProfile() != "gs" {
		return s.rtsp.Stop()
	}

	cfg, err := config.LoadWithMaster(s.masterPath, s.cfgPath, s.defaultPath)
	if err != nil {
		return err
	}

	systemdStates, err := service.Status("rtsp@h265", "rtsp@h264")
	if err != nil {
		return err
	}
	systemdActive := false
	for _, state := range systemdStates {
		if state.Active == "active" {
			systemdActive = true
			break
		}
	}

	localRTSPPeer := strings.EqualFold(strings.TrimSpace(cfg.GSVideo.Peer), "connect://127.0.0.1:5600")
	desiredNative := localRTSPPeer && !systemdActive
	if desiredNative {
		if err := s.rtsp.Start(s.rtspOptions()); err != nil {
			return err
		}
		return nil
	}
	return s.rtsp.Stop()
}

func (s *Server) reconcileCamera() error {
	if s.selectedProfile() != "drone" {
		return s.camera.Stop()
	}

	cfg, err := config.LoadWithMaster(s.masterPath, s.cfgPath, s.defaultPath)
	if err != nil {
		return err
	}
	systemdStates, err := service.Status("fpv-camera.service")
	if err != nil {
		return err
	}
	systemdActive := len(systemdStates) > 0 && systemdStates[0].Active == "active"
	if cfg.Default.CameraEnabled && !systemdActive {
		return s.camera.Start(s.cameraOptions())
	}
	return s.camera.Stop()
}

func rtspServiceState(state rtsp.State) service.State {
	sub := state.Sub
	if state.Error != "" {
		sub = state.Error
	}
	return service.State{
		Unit:      state.Unit,
		Active:    state.Active,
		Sub:       sub,
		Load:      state.Load,
		UnitFile:  state.UnitFile,
		CanReload: state.CanReload,
	}
}

func cameraServiceState(state camera.State) service.State {
	sub := state.Sub
	if state.Error != "" {
		sub = state.Error
	}
	return service.State{
		Unit:      state.Unit,
		Active:    state.Active,
		Sub:       sub,
		Load:      state.Load,
		UnitFile:  state.UnitFile,
		CanReload: state.CanReload,
	}
}

func isSystemdRTSPUnit(unit string) bool {
	return strings.HasPrefix(unit, "rtsp@")
}

func isSystemdCameraUnit(unit string) bool {
	return unit == "fpv-camera.service"
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
