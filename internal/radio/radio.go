package radio

import (
	"os/exec"
	"strings"
	"time"
)

type InterfaceInfo struct {
	Name    string `json:"name"`
	Ethtool string `json:"ethtool"`
	IW      string `json:"iw"`
	Error   string `json:"error,omitempty"`
}

func Inspect(names []string) []InterfaceInfo {
	if len(names) == 0 {
		names = []string{"wlan0"}
	}
	result := make([]InterfaceInfo, 0, len(names))
	for _, name := range names {
		if name == "" {
			continue
		}
		info := InterfaceInfo{Name: name}
		info.Ethtool = run("ethtool", "-i", name)
		info.IW = run("iw", "dev", name, "info")
		if strings.TrimSpace(info.Ethtool) == "" && strings.TrimSpace(info.IW) == "" {
			info.Error = "no ethtool or iw output"
		}
		result = append(result, info)
	}
	return result
}

func run(name string, args ...string) string {
	cmd := exec.Command(name, args...)
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		return "command timed out"
	}
	if err != nil && len(out) == 0 {
		return err.Error()
	}
	return strings.TrimSpace(string(out))
}
