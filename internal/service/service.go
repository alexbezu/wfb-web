package service

import (
	"errors"
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
	"wifibroadcast-gs": "wifibroadcast@gs",
	"rtsp-h265":        "rtsp@h265",
	"rtsp-h264":        "rtsp@h264",
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
	cmd := exec.Command("systemctl", action, unit)
	if out, err := cmd.CombinedOutput(); err != nil {
		return errors.New(strings.TrimSpace(string(out)) + ": " + err.Error())
	}
	return nil
}
