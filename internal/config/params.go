package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

type ParameterConfig struct {
	Files    ConfigFiles              `json:"files"`
	Sections []EffectiveConfigSection `json:"sections"`
}

type ParameterUpdate struct {
	Section string `json:"section"`
	Key     string `json:"key"`
	Value   string `json:"value"`
}

type parsedParam struct {
	section        string
	key            string
	value          string
	inlineComment  string
	leadingComment []string
	order          int
}

type parsedDoc struct {
	params  map[string]parsedParam
	order   []string
	section []string
}

func LoadParameters(masterPath, cfgPath, defaultPath string) (ParameterConfig, error) {
	masterPath = resolveMasterPath(masterPath)
	master := parseConfigFile(masterPath)
	local := parseConfigFile(cfgPath)
	defaults := defaultParamDoc()
	env := parseDefaultFile(defaultPath)

	if masterPath == "" {
		master = parsedDoc{}
	}
	result := ParameterConfig{
		Files: ConfigFiles{Master: masterPath, Local: cfgPath, Default: defaultPath},
	}
	result.Sections = mergeDocs(master, local, defaults, env)
	return result, nil
}

func SaveParameters(masterPath, cfgPath, defaultPath string, updates []ParameterUpdate) error {
	masterPath = resolveMasterPath(masterPath)
	master := parseConfigFile(masterPath)
	local := parseConfigFile(cfgPath)
	defaults := defaultParamDoc()
	env := parseDefaultFile(defaultPath)

	values := effectiveValues(master, local)
	for _, update := range updates {
		section := strings.TrimSpace(update.Section)
		key := strings.TrimSpace(update.Key)
		if section == "" || key == "" {
			return errors.New("section and key are required")
		}
		values[paramID(section, key)] = parsedParam{
			section: section,
			key:     key,
			value:   strings.TrimSpace(update.Value),
		}
	}

	cfgData := renderDiffDoc(master, local, values)
	if err := writeFileWithBackup(cfgPath, cfgData); err != nil {
		return err
	}

	envValues := effectiveValues(defaults, env)
	for _, update := range updates {
		if strings.TrimSpace(update.Section) != "default" {
			continue
		}
		key := strings.TrimSpace(update.Key)
		envValues[paramID("default", key)] = parsedParam{section: "default", key: key, value: strings.TrimSpace(update.Value)}
	}
	return writeFileWithBackup(defaultPath, renderDefaultDiff(defaults, env, envValues))
}

func SaveDiff(masterPath, cfgPath, defaultPath string, cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	updates := []ParameterUpdate{
		{Section: "common", Key: "wifi_channel", Value: fmt.Sprint(cfg.Common.WiFiChannel)},
		{Section: "common", Key: "wifi_region", Value: quotePythonString(cfg.Common.WiFiRegion)},
		{Section: "common", Key: "link_domain", Value: quotePythonString(cfg.Common.LinkDomain)},
		{Section: "base", Key: "ldpc", Value: fmt.Sprint(cfg.Base.LDPC)},
		{Section: "base", Key: "stbc", Value: fmt.Sprint(cfg.Base.STBC)},
		{Section: "base", Key: "bandwidth", Value: fmt.Sprint(cfg.Base.Bandwith)},
		{Section: "base", Key: "mcs_index", Value: fmt.Sprint(cfg.Base.MCSIndex)},
		{Section: "base", Key: "force_vht", Value: pythonBool(cfg.Base.ForceVHT)},
		{Section: "gs_video", Key: "peer", Value: quotePythonString(cfg.GSVideo.Peer)},
		{Section: "default", Key: "WFB_NICS", Value: quoteShellString(cfg.Default.WFBNics)},
		{Section: "default", Key: "RTP_MTU", Value: fmt.Sprint(cfg.Default.RTPMTU)},
		{Section: "default", Key: "RTP_JITTER", Value: fmt.Sprint(cfg.Default.RTPJitter)},
		{Section: "default", Key: "RTSP_PORT", Value: fmt.Sprint(cfg.Default.RTSPPort)},
		{Section: "default", Key: "RTSP_URI", Value: quoteShellString(cfg.Default.RTSPURI)},
	}
	return SaveParameters(masterPath, cfgPath, defaultPath, updates)
}

func mergeDocs(master, local, defaults, env parsedDoc) []EffectiveConfigSection {
	values := effectiveValues(master, local)
	envValues := effectiveValues(defaults, env)
	order := append([]string{}, master.order...)
	order = appendMissing(order, local.order)
	order = append(order, defaults.order...)
	order = appendMissing(order, env.order)

	bySection := map[string][]EffectiveConfigField{}
	for _, id := range order {
		p, ok := values[id]
		base, hasBase := master.params[id]
		if !ok {
			p, ok = envValues[id]
			base, hasBase = defaults.params[id]
		}
		if !ok {
			continue
		}
		source := "master"
		if _, exists := local.params[id]; exists {
			source = "local"
		}
		if p.section == "default" {
			source = "default"
		}
		comment := p.inlineComment
		if comment == "" {
			comment = base.inlineComment
		}
		defaultValue := ""
		if hasBase {
			defaultValue = base.value
		}
		changed := hasBase && !sameValue(p.value, base.value)
		if !hasBase {
			changed = source != "master"
		}
		bySection[p.section] = append(bySection[p.section], EffectiveConfigField{
			Section:      p.section,
			Key:          p.key,
			Value:        p.value,
			DefaultValue: defaultValue,
			Changed:      changed,
			Default:      !changed,
			Source:       source,
			Comment:      comment,
			Editable:     true,
		})
	}

	sections := make([]string, 0, len(bySection))
	for section := range bySection {
		sections = append(sections, section)
	}
	sort.SliceStable(sections, func(i, j int) bool {
		return sectionRank(sections[i]) < sectionRank(sections[j])
	})

	result := make([]EffectiveConfigSection, 0, len(sections))
	for _, section := range sections {
		result = append(result, EffectiveConfigSection{Name: section, Fields: bySection[section]})
	}
	return result
}

func effectiveValues(base, override parsedDoc) map[string]parsedParam {
	values := map[string]parsedParam{}
	for _, id := range base.order {
		values[id] = base.params[id]
	}
	for _, id := range override.order {
		values[id] = override.params[id]
	}
	return values
}

func renderDiffDoc(master, local parsedDoc, values map[string]parsedParam) []byte {
	bySection := map[string][]parsedParam{}
	sectionOrder := []string{}
	for _, id := range appendMissing(master.order, local.order) {
		p, ok := values[id]
		if !ok || p.section == "default" {
			continue
		}
		base, hasBase := master.params[id]
		if hasBase && sameValue(p.value, base.value) {
			continue
		}
		if _, seen := bySection[p.section]; !seen {
			sectionOrder = append(sectionOrder, p.section)
		}
		bySection[p.section] = append(bySection[p.section], p)
	}

	var b strings.Builder
	b.WriteString("# Local wfb-ng overrides. Values equal to master.cfg are omitted.\n")
	for _, section := range sectionOrder {
		fields := bySection[section]
		if len(fields) == 0 {
			continue
		}
		b.WriteString("\n[")
		b.WriteString(section)
		b.WriteString("]\n")
		for _, p := range fields {
			commentSource := p
			if localParam, ok := local.params[paramID(p.section, p.key)]; ok {
				commentSource = localParam
			}
			if masterParam, ok := master.params[paramID(p.section, p.key)]; ok {
				if len(commentSource.leadingComment) == 0 {
					commentSource.leadingComment = masterParam.leadingComment
				}
				if commentSource.inlineComment == "" {
					commentSource.inlineComment = masterParam.inlineComment
				}
			}
			for _, line := range commentSource.leadingComment {
				b.WriteString(line)
				b.WriteByte('\n')
			}
			b.WriteString(p.key)
			b.WriteString(" = ")
			b.WriteString(strings.TrimSpace(p.value))
			if commentSource.inlineComment != "" {
				b.WriteByte(' ')
				b.WriteString(commentSource.inlineComment)
			}
			b.WriteByte('\n')
		}
	}
	return []byte(b.String())
}

func renderDefaultDiff(defaults, local parsedDoc, values map[string]parsedParam) []byte {
	var b strings.Builder
	b.WriteString("# Local wfb-web environment overrides. Values equal to built-in defaults are omitted.\n")
	for _, id := range appendMissing(defaults.order, local.order) {
		p, ok := values[id]
		if !ok {
			continue
		}
		base, hasBase := defaults.params[id]
		if hasBase && sameValue(p.value, base.value) {
			continue
		}
		commentSource := p
		if localParam, ok := local.params[id]; ok {
			commentSource = localParam
		} else if hasBase {
			commentSource = base
		}
		for _, line := range commentSource.leadingComment {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteString(p.key)
		b.WriteByte('=')
		b.WriteString(strings.TrimSpace(p.value))
		if commentSource.inlineComment != "" {
			b.WriteByte(' ')
			b.WriteString(commentSource.inlineComment)
		}
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func parseConfigFile(path string) parsedDoc {
	file, err := os.Open(path)
	if err != nil {
		return parsedDoc{params: map[string]parsedParam{}}
	}
	defer file.Close()
	return parseConfigScanner(bufio.NewScanner(file), false)
}

func parseDefaultFile(path string) parsedDoc {
	file, err := os.Open(path)
	if err != nil {
		return parsedDoc{params: map[string]parsedParam{}}
	}
	defer file.Close()
	return parseConfigScanner(bufio.NewScanner(file), true)
}

func parseConfigScanner(scanner *bufio.Scanner, shell bool) parsedDoc {
	doc := parsedDoc{params: map[string]parsedParam{}}
	section := ""
	if shell {
		section = "default"
	}
	var pending []string
	var current *parsedParam
	depth := 0

	flush := func() {
		if current == nil {
			return
		}
		current.value = strings.TrimSpace(current.value)
		id := paramID(current.section, current.key)
		if _, exists := doc.params[id]; !exists {
			doc.order = append(doc.order, id)
		}
		doc.params[id] = *current
		current = nil
		depth = 0
	}

	for scanner.Scan() {
		raw := scanner.Text()
		trimmed := strings.TrimSpace(raw)
		if current != nil {
			if !isContinuationLine(trimmed, depth) {
				flush()
			}
		}
		if current != nil {
			lineValue, comment := splitInlineComment(raw)
			if comment != "" && current.inlineComment == "" {
				current.inlineComment = comment
			}
			current.value += "\n" + strings.TrimRight(lineValue, " \t")
			depth += bracketDelta(lineValue)
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			pending = append(pending, raw)
			continue
		}
		if !shell && strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]"))
			doc.section = append(doc.section, section)
			pending = nil
			continue
		}
		key, value, ok := strings.Cut(raw, "=")
		if !ok || section == "" {
			pending = nil
			continue
		}
		valuePart, comment := splitInlineComment(value)
		current = &parsedParam{
			section:        section,
			key:            strings.TrimSpace(key),
			value:          strings.TrimSpace(valuePart),
			inlineComment:  comment,
			leadingComment: append([]string{}, pending...),
			order:          len(doc.order),
		}
		pending = nil
		depth = bracketDelta(valuePart)
		if depth <= 0 {
			flush()
		}
	}
	flush()
	return doc
}

func defaultParamDoc() parsedDoc {
	doc := parsedDoc{params: map[string]parsedParam{}}
	for _, p := range []parsedParam{
		{section: "default", key: "WFB_NICS", value: quoteShellString(Defaults().Default.WFBNics), inlineComment: "# radio interfaces"},
		{section: "default", key: "RTP_MTU", value: fmt.Sprint(Defaults().Default.RTPMTU)},
		{section: "default", key: "RTP_JITTER", value: fmt.Sprint(Defaults().Default.RTPJitter)},
		{section: "default", key: "RTSP_PORT", value: fmt.Sprint(Defaults().Default.RTSPPort)},
		{section: "default", key: "RTSP_URI", value: quoteShellString(Defaults().Default.RTSPURI)},
	} {
		id := paramID(p.section, p.key)
		doc.order = append(doc.order, id)
		doc.params[id] = p
	}
	return doc
}

func appendMissing(base []string, extra ...[]string) []string {
	out := append([]string{}, base...)
	seen := map[string]bool{}
	for _, id := range out {
		seen[id] = true
	}
	for _, ids := range extra {
		for _, id := range ids {
			if !seen[id] {
				out = append(out, id)
				seen[id] = true
			}
		}
	}
	return out
}

func paramID(section, key string) string {
	return section + "." + key
}

func sectionRank(section string) int {
	switch section {
	case "common":
		return 0
	case "base":
		return 1
	case "gs_video":
		return 2
	case "default":
		return 3
	default:
		if section == "" {
			return 1000
		}
		return 100 + int([]rune(section)[0])
	}
}

func sameValue(a, b string) bool {
	return normalizeValue(a) == normalizeValue(b)
}

func normalizeValue(value string) string {
	var lines []string
	for _, line := range strings.Split(value, "\n") {
		line, _ = splitInlineComment(line)
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(strings.Fields(strings.Join(lines, " ")), " ")
}

func isContinuationLine(line string, depth int) bool {
	if depth > 0 {
		return true
	}
	if line == "" || strings.HasPrefix(line, "#") {
		return true
	}
	if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
		return false
	}
	key, _, ok := strings.Cut(line, "=")
	if ok && strings.TrimSpace(key) != "" && !strings.ContainsAny(key, " {}[](),:") {
		return false
	}
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "}") || strings.HasPrefix(line, "]")
}

func bracketDelta(value string) int {
	inQuote := rune(0)
	delta := 0
	for _, r := range value {
		switch r {
		case '\'', '"':
			if inQuote == 0 {
				inQuote = r
			} else if inQuote == r {
				inQuote = 0
			}
		case '[', '{', '(':
			if inQuote == 0 {
				delta++
			}
		case ']', '}', ')':
			if inQuote == 0 {
				delta--
			}
		}
	}
	return delta
}

func splitInlineComment(line string) (string, string) {
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
				return strings.TrimRight(line[:i], " \t"), strings.TrimSpace(line[i:])
			}
		}
	}
	return line, ""
}

func quotePythonString(value string) string {
	return strconvQuote(value, "'")
}

func quoteShellString(value string) string {
	return strconvQuote(value, "\"")
}

func strconvQuote(value, quote string) string {
	escaped := strings.ReplaceAll(value, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, quote, "\\"+quote)
	return quote + escaped + quote
}
