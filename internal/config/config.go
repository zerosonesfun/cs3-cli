package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultBaseURL = "https://ctrlshift3.com"
	AppName        = "cs3"
	KeyringService = "com.ctrlshift3.cli"
	KeyringAccount = "api_token"
)

type File struct {
	BaseURL          string `json:"base_url,omitempty"`
	Username         string `json:"username,omitempty"`
	LastSeenPingID   int64  `json:"last_seen_ping_id,omitempty"`
}

func Dir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", AppName), nil
}

func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func CredentialsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials"), nil
}

func Load() (File, error) {
	var out File
	p, err := Path()
	if err != nil {
		return out, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out, nil
		}
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, fmt.Errorf("config: %w", err)
	}
	return out, nil
}

func Save(cfg File) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p, err := Path()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(dir, ".tmp-config-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, p); err != nil {
		return err
	}
	ok = true
	return nil
}

func BaseURL() (string, error) {
	if env := strings.TrimSpace(os.Getenv("CS3_BASE_URL")); env != "" {
		return NormalizeBaseURL(env)
	}
	cfg, err := Load()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		return NormalizeBaseURL(cfg.BaseURL)
	}
	return DefaultBaseURL, nil
}

func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return "", errors.New("base URL is empty")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid base URL %q", raw)
	}
	if u.User != nil {
		return "", errors.New("base URL must not include credentials")
	}
	if u.Path != "" && u.Path != "/" {
		return "", errors.New("base URL must be an origin only (no path)")
	}
	allowInsecure := os.Getenv("CS3_ALLOW_INSECURE") == "1"
	if u.Scheme != "https" && !(allowInsecure && u.Scheme == "http") {
		return "", errors.New("base URL must be https (set CS3_ALLOW_INSECURE=1 for local http)")
	}
	return u.Scheme + "://" + u.Host, nil
}
