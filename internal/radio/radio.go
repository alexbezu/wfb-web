package radio

import (
	"os"
	"os/exec"
	"path/filepath"
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
	bin, err := resolveCommand(name)
	if err != nil {
		return err.Error()
	}
	cmd := exec.Command(bin, args...)
	done := make(chan struct{})
	var out []byte
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

func resolveCommand(name string) (string, error) {
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	for _, dir := range []string{"/usr/local/sbin", "/usr/local/bin", "/usr/sbin", "/usr/bin", "/sbin", "/bin"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}
