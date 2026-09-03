package service

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

type State struct {
	Unit      string `json:"unit"`
	Active    string `json:"active"`
	Sub       string `json:"sub"`
	Load      string `json:"load"`
	UnitFile  string `json:"unit_file"`
	CanReload bool   `json:"can_reload"`
}

var units = map[string]string{
	"wifibroadcast-gs":    "wifibroadcast@gs",
	"wifibroadcast-drone": "wifibroadcast@drone",
	"rtsp-h265":           "rtsp@h265",
	"rtsp-h264":           "rtsp@h264",
	"fpv-camera":          "fpv-camera.service",
}

func AllowedUnit(name string) (string, bool) {
	unit, ok := units[name]
	return unit, ok
}

func Status(names ...string) ([]State, error) {
	states := make([]State, 0, len(names))
	for _, name := range names {
		out, err := exec.Command("systemctl", "show", name, "--property=Id,LoadState,ActiveState,SubState,UnitFileState,CanReload", "--value").Output()
		if err != nil {
			states = append(states, State{Unit: name, Active: "unknown", Sub: strings.TrimSpace(err.Error())})
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		for len(lines) < 6 {
			lines = append(lines, "")
		}
		states = append(states, State{
			Unit:      lines[0],
			Load:      lines[1],
			Active:    lines[2],
			Sub:       lines[3],
			UnitFile:  lines[4],
			CanReload: lines[5] == "yes",
		})
	}
	return states, nil
}

func Run(unit, action string) error {
	switch action {
	case "start", "stop", "restart", "enable", "disable":
	default:
		return errors.New("unsupported service action")
	}
	if action == "enable" || action == "disable" {
		args := append([]string{"systemctl", action}, enableUnits(unit)...)
		cmd := commandWithOptionalSudo(args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return errors.New(strings.TrimSpace(string(out)) + ": " + err.Error())
		}
		return nil
	}
	cmd := exec.Command("systemctl", action, unit)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(out)) + ": " + err.Error())
	}
	return nil
}

func enableUnits(unit string) []string {
	switch strings.TrimSuffix(unit, ".service") {
	case "wifibroadcast@gs":
		return []string{"wifibroadcast.service", "wifibroadcast@gs.service"}
	case "wifibroadcast@drone":
		return []string{"wifibroadcast.service", "wifibroadcast@drone.service"}
	default:
		return []string{unit}
	}
}

func commandWithOptionalSudo(args ...string) *exec.Cmd {
	if os.Geteuid() == 0 {
		return exec.Command(args[0], args[1:]...)
	}
	return exec.Command("sudo", args...)
}
