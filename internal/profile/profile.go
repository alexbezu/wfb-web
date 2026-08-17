package profile

import (
	"bufio"
	"encoding/json"
	"net"
	"os/exec"
	"strings"
	"time"
)

type Selection struct {
	Profile string   `json:"profile"`
	Source  string   `json:"source"`
	Options []Option `json:"options"`
}

type Option struct {
	Profile string `json:"profile"`
	Label   string `json:"label"`
	APIAddr string `json:"api_addr"`
}

var options = []Option{
	{Profile: "gs", Label: "Ground station", APIAddr: "127.0.0.1:8103"},
	{Profile: "drone", Label: "Drone", APIAddr: "127.0.0.1:8102"},
}

func Detect(defaultProfile string) Selection {
	for _, option := range []Option{options[1], options[0]} {
		if profile, ok := readAPIProfile(option.APIAddr); ok && Allowed(profile) {
			return selection(profile, "json-api")
		}
	}
	if profile, ok := activeSystemdProfile(); ok {
		return selection(profile, "systemd")
	}
	if Allowed(defaultProfile) {
		return selection(defaultProfile, "manual")
	}
	return selection("gs", "fallback")
}

func Manual(name string) Selection {
	return selection(name, "manual")
}

func Allowed(name string) bool {
	for _, option := range options {
		if option.Profile == name {
			return true
		}
	}
	return false
}

func selection(name, source string) Selection {
	return Selection{Profile: name, Source: source, Options: options}
}

func readAPIProfile(addr string) (string, bool) {
	conn, err := net.DialTimeout("tcp", addr, 300*time.Millisecond)
	if err != nil {
		return "", false
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", false
	}
	var msg struct {
		Type    string `json:"type"`
		Profile string `json:"profile"`
	}
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return "", false
	}
	return msg.Profile, msg.Type == "settings" && msg.Profile != ""
}

func activeSystemdProfile() (string, bool) {
	for _, name := range []string{"drone", "gs"} {
		cmd := exec.Command("systemctl", "is-active", "--quiet", "wifibroadcast@"+name)
		if err := cmd.Run(); err == nil {
			return name, true
		}
	}

	out, err := exec.Command("systemctl", "list-units", "wifibroadcast@*.service", "--state=active", "--no-legend").Output()
	if err != nil {
		return "", false
	}
	for _, field := range strings.Fields(string(out)) {
		if strings.HasPrefix(field, "wifibroadcast@") && strings.HasSuffix(field, ".service") {
			profile := strings.TrimSuffix(strings.TrimPrefix(field, "wifibroadcast@"), ".service")
			if Allowed(profile) {
				return profile, true
			}
		}
	}
	return "", false
}
