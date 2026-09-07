package rtsp

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	maxStartAttempts = 5
	retryDelay       = 2 * time.Second
	startupGrace     = 350 * time.Millisecond
)

type Options struct {
	Codec   string `json:"codec"`
	MTU     int    `json:"mtu"`
	Port    int    `json:"port"`
	URI     string `json:"uri"`
	Latency int    `json:"latency"`
	RTPPort int    `json:"rtp_port"`
}

type State struct {
	Unit      string  `json:"unit"`
	Active    string  `json:"active"`
	Sub       string  `json:"sub"`
	Load      string  `json:"load"`
	UnitFile  string  `json:"unit_file"`
	CanReload bool    `json:"can_reload"`
	Options   Options `json:"options"`
	URL       string  `json:"url"`
	Error     string  `json:"error,omitempty"`
	Native    bool    `json:"native"`
}

type Manager struct {
	mu       sync.Mutex
	running  bool
	stopping bool
	opts     Options
	lastErr  string
	stop     chan struct{}
	done     chan struct{}
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Status(opts Options) State {
	m.mu.Lock()
	defer m.mu.Unlock()

	state := State{
		Unit:      "wfb-web-rtsp",
		Active:    "inactive",
		Sub:       "dead",
		Load:      "loaded",
		CanReload: true,
		Options:   opts.withDefaults(),
		Native:    nativeAvailable(),
	}
	if m.running {
		state.Active = "active"
		state.Sub = "running"
		state.Options = m.opts
	}
	if state.Native {
		state.UnitFile = "native"
	} else {
		state.UnitFile = "unavailable"
	}
	if state.Options.Port > 0 && state.Options.URI != "" {
		state.URL = fmt.Sprintf("rtsp://127.0.0.1:%d%s", state.Options.Port, state.Options.URI)
	}
	if m.lastErr != "" {
		state.Error = m.lastErr
	}
	return state
}

func (m *Manager) Start(opts Options) error {
	opts = opts.withDefaults()
	if err := opts.validate(); err != nil {
		return err
	}
	if !nativeAvailable() {
		return errors.New("native GStreamer RTSP support is not built; rebuild with -tags gstreamer and gst-rtsp-server development files")
	}

	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = true
	m.stopping = false
	m.opts = opts
	m.lastErr = ""
	stop := make(chan struct{})
	done := make(chan struct{})
	m.stop = stop
	m.done = done
	m.mu.Unlock()

	go func() {
		defer close(done)
		lastErr := ""
	retryLoop:
		for attempt := 1; attempt <= maxStartAttempts; attempt++ {
			if stopped(stop) {
				break
			}

			m.mu.Lock()
			m.lastErr = ""
			m.mu.Unlock()

			err := runNative(opts)
			if stopped(stop) {
				break
			}
			if err != nil {
				lastErr = err.Error()
			} else {
				lastErr = "rtsp server exited"
			}
			if attempt == maxStartAttempts {
				break
			}

			m.mu.Lock()
			m.lastErr = fmt.Sprintf("rtsp server attempt %d/%d failed: %s; retrying in %s", attempt, maxStartAttempts, lastErr, retryDelay)
			m.mu.Unlock()

			select {
			case <-stop:
				break retryLoop
			case <-time.After(retryDelay):
			}
		}

		m.mu.Lock()
		m.running = false
		m.stopping = false
		if stopped(stop) {
			m.lastErr = ""
		} else if lastErr != "" {
			m.lastErr = fmt.Sprintf("rtsp server failed after %d attempts: %s", maxStartAttempts, lastErr)
		}
		m.mu.Unlock()
	}()

	select {
	case <-done:
		m.mu.Lock()
		err := m.lastErr
		m.mu.Unlock()
		if err != "" {
			return errors.New(err)
		}
	case <-time.After(startupGrace):
	}
	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	running := m.running
	stop := m.stop
	done := m.done
	if running && !m.stopping {
		close(stop)
		m.stopping = true
	}
	m.mu.Unlock()
	if !running {
		return nil
	}
	stopNative()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		return errors.New("timed out waiting for rtsp server to stop")
	}
	return nil
}

func stopped(stop <-chan struct{}) bool {
	select {
	case <-stop:
		return true
	default:
		return false
	}
}

func (m *Manager) Restart(opts Options) error {
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start(opts)
}

func (o Options) withDefaults() Options {
	if o.Codec == "" {
		o.Codec = "h265"
	}
	if o.MTU == 0 {
		o.MTU = 1400
	}
	if o.Port == 0 {
		o.Port = 8554
	}
	if o.URI == "" {
		o.URI = "/wfb"
	}
	if o.RTPPort == 0 {
		o.RTPPort = 5600
	}
	return o
}

func (o Options) validate() error {
	if o.Codec != "h264" && o.Codec != "h265" {
		return errors.New("codec must be h264 or h265, but " + o.Codec)
	}
	if o.MTU <= 0 || o.Port <= 0 || o.RTPPort <= 0 {
		return errors.New("mtu, rtsp port, and rtp port must be positive")
	}
	if o.URI == "" || o.URI[0] != '/' {
		return errors.New("rtsp uri must start with /")
	}
	return nil
}
