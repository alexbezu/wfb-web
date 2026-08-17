package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Common  CommonConfig  `json:"common"`
	Base    BaseConfig    `json:"base"`
	GSVideo GSVideoConfig `json:"gs_video"`
	Default DefaultConfig `json:"default"`
}

type CommonConfig struct {
	WiFiChannel int    `json:"wifi_channel"`
	WiFiRegion  string `json:"wifi_region"`
	LinkDomain  string `json:"link_domain"`
}

type BaseConfig struct {
	LDPC     int  `json:"ldpc"`
	STBC     int  `json:"stbc"`
	Bandwith int  `json:"bandwidth"`
	MCSIndex int  `json:"mcs_index"`
	ForceVHT bool `json:"force_vht"`
}

type GSVideoConfig struct {
	Peer string `json:"peer"`
}

type DefaultConfig struct {
	WFBNics   string `json:"wfb_nics"`
	RTPMTU    int    `json:"rtp_mtu"`
	RTPJitter int    `json:"rtp_jitter"`
	RTSPPort  int    `json:"rtsp_port"`
	RTSPURI   string `json:"rtsp_uri"`
}

func Load(cfgPath, defaultPath string) (Config, error) {
	cfg := Defaults()
	if err := loadINI(cfgPath, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}
	if err := loadDefault(defaultPath, &cfg); err != nil && !errors.Is(err, os.ErrNotExist) {
		return cfg, err
	}
	return cfg, nil
}

func Save(cfgPath, defaultPath string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	if err := writeFileWithBackup(cfgPath, renderINI(cfg)); err != nil {
		return err
	}
	return writeFileWithBackup(defaultPath, renderDefault(cfg))
}

func Defaults() Config {
	return Config{
		Common:  CommonConfig{WiFiChannel: 161, WiFiRegion: "BO", LinkDomain: "default"},
		Base:    BaseConfig{LDPC: 0, STBC: 0, Bandwith: 20, MCSIndex: 1, ForceVHT: false},
		GSVideo: GSVideoConfig{Peer: "connect://127.0.0.1:5600"},
		Default: DefaultConfig{WFBNics: "wlan0", RTPMTU: 1400, RTPJitter: 0, RTSPPort: 8554, RTSPURI: "/wfb"},
	}
}

func (c Config) Validate() error {
	if c.Common.WiFiChannel <= 0 {
		return errors.New("wifi_channel must be positive")
	}
	if c.Common.WiFiRegion == "" {
		return errors.New("wifi_region is required")
	}
	if c.Common.LinkDomain == "" {
		return errors.New("link_domain is required")
	}
	if c.Base.Bandwith != 20 && c.Base.Bandwith != 40 {
		return errors.New("bandwidth must be 20 or 40")
	}
	if c.Base.MCSIndex < 0 || c.Base.MCSIndex > 11 {
		return errors.New("mcs_index must be between 0 and 11")
	}
	if c.Base.LDPC < 0 || c.Base.LDPC > 1 || c.Base.STBC < 0 || c.Base.STBC > 3 {
		return errors.New("invalid ldpc or stbc value")
	}
	if !strings.HasPrefix(c.GSVideo.Peer, "connect://") {
		return errors.New("gs_video.peer must start with connect://")
	}
	if c.Default.WFBNics == "" {
		return errors.New("WFB_NICS is required")
	}
	if c.Default.RTPMTU <= 0 || c.Default.RTSPPort <= 0 {
		return errors.New("RTP_MTU and RTSP_PORT must be positive")
	}
	if c.Default.RTSPURI == "" || !strings.HasPrefix(c.Default.RTSPURI, "/") {
		return errors.New("RTSP_URI must start with /")
	}
	return nil
}

func loadINI(path string, cfg *Config) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := stripComment(strings.TrimSpace(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		setINIValue(cfg, section, strings.TrimSpace(key), strings.TrimSpace(value))
	}
	return scanner.Err()
}

func setINIValue(cfg *Config, section, key, value string) {
	switch section + "." + key {
	case "common.wifi_channel":
		cfg.Common.WiFiChannel = intValue(value, cfg.Common.WiFiChannel)
	case "common.wifi_region":
		cfg.Common.WiFiRegion = stringValue(value)
	case "common.link_domain":
		cfg.Common.LinkDomain = stringValue(value)
	case "base.ldpc":
		cfg.Base.LDPC = intValue(value, cfg.Base.LDPC)
	case "base.stbc":
		cfg.Base.STBC = intValue(value, cfg.Base.STBC)
	case "base.bandwidth":
		cfg.Base.Bandwith = intValue(value, cfg.Base.Bandwith)
	case "base.mcs_index":
		cfg.Base.MCSIndex = intValue(value, cfg.Base.MCSIndex)
	case "base.force_vht":
		cfg.Base.ForceVHT = boolValue(value, cfg.Base.ForceVHT)
	case "gs_video.peer":
		cfg.GSVideo.Peer = stringValue(value)
	}
}

func loadDefault(path string, cfg *Config) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = stringValue(strings.TrimSpace(value))
		switch strings.TrimSpace(key) {
		case "WFB_NICS":
			cfg.Default.WFBNics = value
		case "RTP_MTU":
			cfg.Default.RTPMTU = intValue(value, cfg.Default.RTPMTU)
		case "RTP_JITTER":
			cfg.Default.RTPJitter = intValue(value, cfg.Default.RTPJitter)
		case "RTSP_PORT":
			cfg.Default.RTSPPort = intValue(value, cfg.Default.RTSPPort)
		case "RTSP_URI":
			cfg.Default.RTSPURI = value
		}
	}
	return scanner.Err()
}

func renderINI(c Config) []byte {
	return []byte(fmt.Sprintf(`[common]
wifi_channel = %d
wifi_region = %q
link_domain = %q

[base]
ldpc = %d
stbc = %d
bandwidth = %d
mcs_index = %d
force_vht = %s

[gs_video]
peer = %q
`, c.Common.WiFiChannel, c.Common.WiFiRegion, c.Common.LinkDomain,
		c.Base.LDPC, c.Base.STBC, c.Base.Bandwith, c.Base.MCSIndex, pythonBool(c.Base.ForceVHT),
		c.GSVideo.Peer))
}

func renderDefault(c Config) []byte {
	return []byte(fmt.Sprintf(`WFB_NICS=%q

RTP_MTU=%d
RTP_JITTER=%d
RTSP_PORT=%d
RTSP_URI=%q
`, c.Default.WFBNics, c.Default.RTPMTU, c.Default.RTPJitter, c.Default.RTSPPort, c.Default.RTSPURI))
}

func writeFileWithBackup(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", old, 0o644); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func stripComment(line string) string {
	inQuote := rune(0)
	for i, r := range line {
		switch r {
		case '\'', '"':
			if inQuote == 0 {
				inQuote = r
			} else if inQuote == r {
				inQuote = 0
			}
		case '#':
			if inQuote == 0 {
				return strings.TrimSpace(line[:i])
			}
		}
	}
	return line
}

func stringValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

func intValue(value string, fallback int) int {
	i, err := strconv.Atoi(stringValue(value))
	if err != nil {
		return fallback
	}
	return i
}

func boolValue(value string, fallback bool) bool {
	switch strings.ToLower(stringValue(value)) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return fallback
	}
}

func pythonBool(value bool) string {
	if value {
		return "True"
	}
	return "False"
}
