package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	if err := os.WriteFile(cfgPath, []byte(`[common]
wifi_channel = 165
wifi_region = 'US'
link_domain = 'lab'

[base]
ldpc = 1
stbc = 2
bandwidth = 40
mcs_index = 4
force_vht = True

[gs_video]
peer = 'connect://239.50.50.50:5600'
`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(defaultPath, []byte(`WFB_NICS="wlan0 wlan1"
RTP_MTU=1300
RTP_JITTER=10
RTSP_PORT=8555
RTSP_URI="/live"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath, defaultPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Common.WiFiChannel != 165 || cfg.Common.WiFiRegion != "US" || cfg.Common.LinkDomain != "lab" {
		t.Fatalf("unexpected common config: %+v", cfg.Common)
	}
	if cfg.Base.Bandwith != 40 || cfg.Base.MCSIndex != 4 || !cfg.Base.ForceVHT {
		t.Fatalf("unexpected base config: %+v", cfg.Base)
	}
	if cfg.GSVideo.Peer != "connect://239.50.50.50:5600" {
		t.Fatalf("unexpected peer: %s", cfg.GSVideo.Peer)
	}
	if cfg.Default.WFBNics != "wlan0 wlan1" || cfg.Default.RTSPURI != "/live" {
		t.Fatalf("unexpected default config: %+v", cfg.Default)
	}
}

func TestSaveCreatesBackups(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	if err := os.WriteFile(cfgPath, []byte("old cfg"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultPath, []byte("old default"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Save(cfgPath, defaultPath, Defaults()); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(cfgPath + ".bak"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(defaultPath + ".bak"); err != nil {
		t.Fatal(err)
	}
}
