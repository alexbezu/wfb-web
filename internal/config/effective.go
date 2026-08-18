package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type EffectiveConfig struct {
	Files    ConfigFiles              `json:"files"`
	Sections []EffectiveConfigSection `json:"sections"`
}

type ConfigFiles struct {
	Master  string `json:"master"`
	Local   string `json:"local"`
	Default string `json:"default"`
}

type EffectiveConfigSection struct {
	Name   string                 `json:"name"`
	Fields []EffectiveConfigField `json:"fields"`
}

type EffectiveConfigField struct {
	Section      string `json:"section"`
	Key          string `json:"key"`
	Value        string `json:"value"`
	DefaultValue string `json:"default_value"`
	Default      bool   `json:"default"`
	Changed      bool   `json:"changed"`
	Source       string `json:"source"`
	Comment      string `json:"comment"`
	Editable     bool   `json:"editable"`
}

type configValue struct {
	section string
	key     string
	value   string
	source  string
	order   int
}

func LoadEffective(masterPath, cfgPath, defaultPath string) (EffectiveConfig, error) {
	params, err := LoadParameters(masterPath, cfgPath, defaultPath)
	if err != nil {
		return EffectiveConfig{}, err
	}
	return EffectiveConfig(params), nil
}

func addValues(dst map[string]configValue, order *[]string, values []configValue) {
	for _, value := range values {
		id := value.section + "." + value.key
		if _, ok := dst[id]; !ok {
			*order = append(*order, id)
		}
		dst[id] = value
	}
}

func readConfigValues(path, source string) []configValue {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var values []configValue
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
		if !ok || section == "" {
			continue
		}
		values = append(values, configValue{
			section: section,
			key:     strings.TrimSpace(key),
			value:   strings.TrimSpace(value),
			source:  source,
			order:   len(values),
		})
	}
	return values
}

func readDefaultValues(path, source string) []configValue {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()

	var values []configValue
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
		values = append(values, configValue{
			section: "default",
			key:     strings.TrimSpace(key),
			value:   strings.TrimSpace(value),
			source:  source,
			order:   len(values),
		})
	}
	return values
}

func resolveMasterPath(path string) string {
	if path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	for _, candidate := range []string{
		"/usr/lib/python3/dist-packages/wfb_ng/conf/master.cfg",
		"/usr/local/lib/python3/dist-packages/wfb_ng/conf/master.cfg",
		"/usr/local/lib/python3.11/dist-packages/wfb_ng/conf/master.cfg",
		"/usr/local/lib/python3.12/dist-packages/wfb_ng/conf/master.cfg",
		"/usr/local/lib/python3.13/dist-packages/wfb_ng/conf/master.cfg",
		filepath.Clean("../wfb-ng/wfb_ng/conf/master.cfg"),
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
