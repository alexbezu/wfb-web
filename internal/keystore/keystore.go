package keystore

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"
)

const maxKeySize = 1024 * 1024

type Info struct {
	Profile string `json:"profile"`
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Size    int64  `json:"size"`
	Hash    string `json:"hash"`
	ModTime string `json:"mod_time"`
}

func Read(profile string) (Info, error) {
	path, err := pathForProfile(profile)
	if err != nil {
		return Info{}, err
	}
	return info(profile, path)
}

func Save(profile string, reader io.Reader) (Info, error) {
	path, err := pathForProfile(profile)
	if err != nil {
		return Info{}, err
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxKeySize+1))
	if err != nil {
		return Info{}, err
	}
	if len(data) == 0 {
		return Info{}, errors.New("key file is empty")
	}
	if len(data) > maxKeySize {
		return Info{}, errors.New("key file is too large")
	}
	if old, err := os.ReadFile(path); err == nil {
		if err := os.WriteFile(path+".bak", old, 0o600); err != nil {
			return Info{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Info{}, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return Info{}, err
	}
	return info(profile, path)
}

func pathForProfile(profile string) (string, error) {
	switch profile {
	case "gs":
		return "/etc/gs.key", nil
	case "drone":
		return "/etc/drone.key", nil
	default:
		return "", errors.New("key upload is supported only for gs and drone profiles")
	}
}

func info(profile, path string) (Info, error) {
	result := Info{Profile: profile, Path: path}
	stat, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result, nil
		}
		return Info{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	sum := sha256.Sum256(data)
	result.Exists = true
	result.Size = stat.Size()
	result.Hash = hex.EncodeToString(sum[:])[:12]
	result.ModTime = stat.ModTime().Format(time.RFC3339)
	return result, nil
}
