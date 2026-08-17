package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"sort"
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
	Section string `json:"section"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Default bool   `json:"default"`
	Source  string `json:"source"`
}

type configValue struct {
	section string
	key     string
	value   string
	source  string
	order   int
}

func LoadEffective(masterPath, cfgPath, defaultPath string) (EffectiveConfig, error) {
	masterPath = resolveMasterPath(masterPath)
	values := map[string]configValue{}
	order := []string{}

	if masterPath != "" {
		addValues(values, &order, readConfigValues(masterPath, "master"))
	}

	localValues := readConfigValues(cfgPath, "local")
	if len(localValues) == 0 {
		if _, err := os.Stat(cfgPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return EffectiveConfig{}, err
		}
	}
	addValues(values, &order, localValues)
	addValues(values, &order, readDefaultValues(defaultPath, "default"))

	bySection := map[string][]EffectiveConfigField{}
	for _, id := range order {
		item := values[id]
		bySection[item.section] = append(bySection[item.section], EffectiveConfigField{
			Section: item.section,
			Key:     item.key,
			Value:   item.value,
			Default: item.source == "master",
			Source:  item.source,
		})
	}

	sectionNames := make([]string, 0, len(bySection))
	for name := range bySection {
		sectionNames = append(sectionNames, name)
	}
	sort.SliceStable(sectionNames, func(i, j int) bool {
		if sectionNames[i] == "common" {
			return true
		}
		if sectionNames[j] == "common" {
			return false
		}
		return sectionNames[i] < sectionNames[j]
	})

	result := EffectiveConfig{
		Files: ConfigFiles{Master: masterPath, Local: cfgPath, Default: defaultPath},
	}
	for _, name := range sectionNames {
		result.Sections = append(result.Sections, EffectiveConfigSection{Name: name, Fields: bySection[name]})
	}
	return result, nil
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
