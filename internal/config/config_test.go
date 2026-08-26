package config

import (
	"os"
	"path/filepath"
	"strings"
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

	if err := os.WriteFile(defaultPath, []byte(`WFB_WEB_PROFILE=drone
WFB_NICS="wlan0 wlan1"
RTP_MTU=1300
RTP_JITTER=10
RTSP_PORT=8555
RTSP_URI="/live"
WFB_WEB_RTSP_CODEC="h264" # wfb-web native RTSP codec
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
	if cfg.Default.Profile != "drone" || cfg.Default.WFBNics != "wlan0 wlan1" || cfg.Default.RTSPURI != "/live" {
		t.Fatalf("unexpected default config: %+v", cfg.Default)
	}
	if cfg.Default.RTSPCodec != "h264" {
		t.Fatalf("unexpected rtsp codec: %s", cfg.Default.RTSPCodec)
	}
}

func TestLoadParametersDisplaysDefaultStringsUnquoted(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	if err := os.WriteFile(defaultPath, []byte(`RTSP_URI="/live"
WFB_WEB_RTSP_CODEC="h264" # wfb-web native RTSP codec
`), 0o644); err != nil {
		t.Fatal(err)
	}

	params, err := LoadParameters("", cfgPath, defaultPath)
	if err != nil {
		t.Fatal(err)
	}

	fields := map[string]EffectiveConfigField{}
	for _, section := range params.Sections {
		for _, field := range section.Fields {
			fields[field.Key] = field
		}
	}
	if fields["RTSP_URI"].Value != "/live" {
		t.Fatalf("expected unquoted RTSP_URI, got %q", fields["RTSP_URI"].Value)
	}
	if fields["WFB_WEB_RTSP_CODEC"].Value != "h264" {
		t.Fatalf("expected unquoted codec, got %q", fields["WFB_WEB_RTSP_CODEC"].Value)
	}
	if fields["WFB_WEB_RTSP_CODEC"].Comment != "# wfb-web native RTSP codec" {
		t.Fatalf("expected codec comment to be split, got %q", fields["WFB_WEB_RTSP_CODEC"].Comment)
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

func TestSaveParametersWritesDiffAndPreservesLocalComments(t *testing.T) {
	dir := t.TempDir()
	masterPath := filepath.Join(dir, "master.cfg")
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	if err := os.WriteFile(masterPath, []byte(`[common]
wifi_channel = 165 # master channel comment
wifi_region = 'BO'

[base]
mcs_index = 1 # master mcs comment
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`[common]
# keep the operator note
wifi_channel = 161 # keep local inline
wifi_region = 'US'

[base]
mcs_index = 2
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaultPath, []byte(`WFB_NICS="wlan0 wlan1"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := SaveParameters(masterPath, cfgPath, defaultPath, []ParameterUpdate{
		{Section: "common", Key: "wifi_region", Value: "'BO'"},
		{Section: "base", Key: "mcs_index", Value: "3"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "# keep the operator note\nwifi_channel = 161 # keep local inline") {
		t.Fatalf("local comments were not preserved:\n%s", got)
	}
	if strings.Contains(got, "wifi_region") {
		t.Fatalf("default-valued override should be omitted:\n%s", got)
	}
	if !strings.Contains(got, "mcs_index = 3 # master mcs comment") {
		t.Fatalf("new override should copy master inline comment:\n%s", got)
	}
}

func TestSaveParametersQuotesDefaultStrings(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	err := SaveParameters("", cfgPath, defaultPath, []ParameterUpdate{
		{Section: "default", Key: "WFB_WEB_RTSP_CODEC", Value: "h264"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, `WFB_WEB_RTSP_CODEC="h264"`) {
		t.Fatalf("codec should be shell-quoted:\n%s", got)
	}
}

func TestSaveParametersWritesSavedProfile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	err := SaveParameters("", cfgPath, defaultPath, []ParameterUpdate{
		{Section: "default", Key: "WFB_WEB_PROFILE", Value: "drone"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "WFB_WEB_PROFILE=drone # saved wfb-web profile") {
		t.Fatalf("saved profile should be written unquoted with comment:\n%s", got)
	}
}

func TestSaveParametersKeepsDefaultWFBNics(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	err := SaveParameters("", cfgPath, defaultPath, []ParameterUpdate{
		{Section: "default", Key: "WFB_NICS", Value: "wlan0"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "WFB_NICS=wlan0 # radio interfaces") {
		t.Fatalf("default WFB_NICS should stay present and commented:\n%s", got)
	}
}

func TestSaveParametersKeepsRequiredDefaultEnvLines(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	err := SaveParameters("", cfgPath, defaultPath, []ParameterUpdate{
		{Section: "default", Key: "WFB_WEB_CAMERA_ENABLED", Value: "false"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{
		"WFB_NICS=wlan0 # radio interfaces",
		"RTP_MTU=1400",
		"RTSP_PORT=8554",
		`RTSP_URI="/wfb"`,
		"RTP_JITTER=0",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("required default env line %q missing:\n%s", want, got)
		}
	}
}

func TestSaveParametersWritesSingleWFBNicUnquoted(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	err := SaveParameters("", cfgPath, defaultPath, []ParameterUpdate{
		{Section: "default", Key: "WFB_NICS", Value: "wlan1"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "WFB_NICS=wlan1 # radio interfaces") {
		t.Fatalf("single WFB_NICS should be unquoted and commented:\n%s", got)
	}
	if strings.Contains(got, `WFB_NICS="wlan1"`) {
		t.Fatalf("single WFB_NICS should not be quoted:\n%s", got)
	}
}

func TestSaveParametersFallsBackForEmptyWFBNics(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	err := SaveParameters("", cfgPath, defaultPath, []ParameterUpdate{
		{Section: "default", Key: "WFB_NICS", Value: ""},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "WFB_NICS= ") || strings.Contains(got, "WFB_NICS=\n") {
		t.Fatalf("empty WFB_NICS should not be written:\n%s", got)
	}
	if !strings.Contains(got, "WFB_NICS=wlan0 # radio interfaces") {
		t.Fatalf("empty WFB_NICS should fall back to wlan0:\n%s", got)
	}
}

func TestSaveParametersNormalizesExistingWFBNics(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wifibroadcast.cfg")
	defaultPath := filepath.Join(dir, "wifibroadcast")

	if err := os.WriteFile(defaultPath, []byte(`WFB_NICS="wlan0"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := SaveParameters("", cfgPath, defaultPath, []ParameterUpdate{
		{Section: "default", Key: "RTP_MTU", Value: "1300"},
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(defaultPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "WFB_NICS=wlan0 # radio interfaces") {
		t.Fatalf("existing default WFB_NICS should be normalized and commented:\n%s", got)
	}
	if strings.Contains(got, `WFB_NICS="wlan0"`) {
		t.Fatalf("existing default WFB_NICS should not remain quoted:\n%s", got)
	}
}
