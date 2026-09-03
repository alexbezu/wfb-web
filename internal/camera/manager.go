package camera

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Options struct {
	Enabled   bool   `json:"enabled"`
	Source    string `json:"source"`
	Device    string `json:"device"`
	RTSPURL   string `json:"rtsp_url"`
	Codec     string `json:"codec"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Framerate int    `json:"framerate"`
	Bitrate   int    `json:"bitrate"`
	MTU       int    `json:"mtu"`
	Pattern   string `json:"pattern"`
}

type State struct {
	Unit      string  `json:"unit"`
	Active    string  `json:"active"`
	Sub       string  `json:"sub"`
	Load      string  `json:"load"`
	UnitFile  string  `json:"unit_file"`
	CanReload bool    `json:"can_reload"`
	Options   Options `json:"options"`
	Output    string  `json:"output"`
	Command   string  `json:"command"`
	Error     string  `json:"error,omitempty"`
}

type Manager struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
	opts    Options
	cmdline []string
	lastErr string
	done    chan struct{}
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Status(opts Options) State {
	m.mu.Lock()
	defer m.mu.Unlock()

	opts = opts.withDefaults()
	state := State{
		Unit:      "wfb-web-camera",
		Active:    "inactive",
		Sub:       "dead",
		Load:      "loaded",
		UnitFile:  "native",
		CanReload: true,
		Options:   opts,
		Output:    fmt.Sprintf("udp://%s:%d", opts.Host, opts.Port),
		Command:   strings.Join(buildCommand(opts), " "),
	}
	if m.running {
		state.Active = "active"
		state.Sub = "running"
		state.Options = m.opts
		state.Output = fmt.Sprintf("udp://%s:%d", m.opts.Host, m.opts.Port)
		state.Command = strings.Join(m.cmdline, " ")
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
	cmdline := buildCommand(opts)
	if _, err := exec.LookPath(cmdline[0]); err != nil {
		return err
	}

	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, cmdline[0], cmdline[1:]...)
	output := &tailBuffer{limit: 64 * 1024}
	cmd.Stdout = output
	cmd.Stderr = output
	done := make(chan struct{})
	m.cancel = cancel
	m.running = true
	m.opts = opts
	m.cmdline = cmdline
	m.lastErr = ""
	m.done = done
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		m.mu.Lock()
		m.running = false
		m.cancel = nil
		m.lastErr = err.Error()
		close(done)
		m.mu.Unlock()
		cancel()
		return err
	}

	go func() {
		defer close(done)
		err := cmd.Wait()
		m.mu.Lock()
		m.running = false
		m.cancel = nil
		if err != nil && ctx.Err() == nil {
			m.lastErr = commandError(err, output.String())
		} else if err == nil && ctx.Err() == nil {
			m.lastErr = "camera pipeline exited"
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
	case <-time.After(350 * time.Millisecond):
	}
	return nil
}

func (m *Manager) Stop() error {
	m.mu.Lock()
	running := m.running
	cancel := m.cancel
	done := m.done
	m.mu.Unlock()
	if !running {
		return nil
	}
	if cancel != nil {
		cancel()
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		return errors.New("timed out waiting for camera pipeline to stop")
	}
	return nil
}

func (m *Manager) Restart(opts Options) error {
	if err := m.Stop(); err != nil {
		return err
	}
	return m.Start(opts)
}

func (o Options) withDefaults() Options {
	if o.Source == "" {
		o.Source = "test"
	}
	if o.Device == "" {
		o.Device = "/dev/video0"
	}
	if o.RTSPURL == "" {
		o.RTSPURL = "rtsp://127.0.0.1:8555/camera"
	}
	if o.Codec == "" {
		o.Codec = "h264"
	}
	if o.Host == "" {
		o.Host = "127.0.0.1"
	}
	if o.Port == 0 {
		o.Port = 5602
	}
	if o.Width == 0 {
		o.Width = 1280
	}
	if o.Height == 0 {
		o.Height = 720
	}
	if o.Framerate == 0 {
		o.Framerate = 30
	}
	if o.Bitrate == 0 {
		o.Bitrate = 2500
	}
	if o.MTU == 0 {
		o.MTU = 1400
	}
	if o.Pattern == "" {
		o.Pattern = "smpte"
	}
	return o
}

func (o Options) validate() error {
	if o.Source != "test" && o.Source != "rtsp" && o.Source != "v4l2-h264" {
		return errors.New("camera source must be test, rtsp, or v4l2-h264")
	}
	if o.Codec != "h264" && o.Codec != "h265" {
		return errors.New("camera codec must be h264 or h265")
	}
	if o.Source == "v4l2-h264" && o.Codec != "h264" {
		return errors.New("v4l2-h264 source requires h264 codec")
	}
	if o.Source == "rtsp" && o.RTSPURL == "" {
		return errors.New("rtsp camera source requires rtsp url")
	}
	if o.Host == "" || o.Port <= 0 || o.Width <= 0 || o.Height <= 0 || o.Framerate <= 0 || o.Bitrate <= 0 || o.MTU <= 0 {
		return errors.New("camera host, port, width, height, framerate, bitrate, and mtu are required")
	}
	return nil
}

func buildCommand(opts Options) []string {
	opts = opts.withDefaults()
	args := []string{"gst-launch-1.0", "-v"}
	switch opts.Source {
	case "rtsp":
		depayloader := "rtph264depay"
		parser := "h264parse"
		if opts.Codec == "h265" {
			depayloader = "rtph265depay"
			parser = "h265parse"
		}
		args = append(args,
			"rtspsrc", "protocols=tcp", "latency=0", "location="+opts.RTSPURL,
			"!", depayloader,
			"!", parser, "disable-passthrough=true",
		)
	case "v4l2-h264":
		args = append(args,
			"v4l2src", "do-timestamp=true", "io-mode=mmap", "device="+opts.Device,
			"!", fmt.Sprintf("video/x-h264,profile=high,width=%d,height=%d,framerate=%d/1,stream-format=byte-stream", opts.Width, opts.Height, opts.Framerate),
			"!", "h264parse", "disable-passthrough=true",
		)
	default:
		args = append(args,
			"videotestsrc", "is-live=true", "pattern="+opts.Pattern,
			"!", fmt.Sprintf("video/x-raw,width=%d,height=%d,framerate=%d/1", opts.Width, opts.Height, opts.Framerate),
			"!", "videoconvert",
		)
		if opts.Codec == "h265" {
			args = append(args, "!", "x265enc", "tune=zerolatency", "speed-preset=ultrafast", fmt.Sprintf("key-int-max=%d", opts.Framerate), fmt.Sprintf("bitrate=%d", opts.Bitrate))
		} else {
			args = append(args, "!", "x264enc", "tune=zerolatency", "speed-preset=ultrafast", fmt.Sprintf("key-int-max=%d", opts.Framerate), fmt.Sprintf("bitrate=%d", opts.Bitrate))
		}
	}
	payloader := "rtph264pay"
	if opts.Codec == "h265" {
		payloader = "rtph265pay"
	}
	args = append(args,
		"!", payloader, "pt=96", "config-interval=1", fmt.Sprintf("mtu=%d", opts.MTU), "aggregate-mode=zero-latency",
		"!", "udpsink", "host="+opts.Host, fmt.Sprintf("port=%d", opts.Port), "sync=false", "async=false",
	)
	return args
}

func commandError(err error, output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return err.Error()
	}
	lines := strings.Split(output, "\n")
	if len(lines) > 12 {
		lines = lines[len(lines)-12:]
	}
	return err.Error() + ": " + strings.Join(lines, "\n")
}

type tailBuffer struct {
	limit int
	data  []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	return string(b.data)
}
