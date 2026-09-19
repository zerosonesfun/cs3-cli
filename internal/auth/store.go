package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zerosonesfun/cs3-cli/internal/config"
	"github.com/zalando/go-keyring"
)

var errMissing = errors.New("not logged in")

func SaveToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("empty token")
	}
	err := keyring.Set(config.KeyringService, config.KeyringAccount, token)
	if err == nil {
		_ = clearFileToken()
		return nil
	}
	path, pathErr := config.CredentialsPath()
	if pathErr == nil {
		fmt.Fprintf(os.Stderr, "Note: system keychain unavailable; storing token in %s (mode 0600).\n", path)
	} else {
		fmt.Fprintln(os.Stderr, "Note: system keychain unavailable; storing token in a local credentials file.")
	}
	return saveFileToken(token)
}

func LoadToken() (string, error) {
	tok, err := keyring.Get(config.KeyringService, config.KeyringAccount)
	if err == nil && strings.TrimSpace(tok) != "" {
		return strings.TrimSpace(tok), nil
	}
	return loadFileToken()
}

func ClearToken() error {
	_ = keyring.Delete(config.KeyringService, config.KeyringAccount)
	return clearFileToken()
}

func RequireToken() (string, error) {
	tok, err := LoadToken()
	if err != nil || tok == "" {
		return "", fmt.Errorf("%w (run: cs3 login)", errMissing)
	}
	return tok, nil
}

func saveFileToken(token string) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	p, err := config.CredentialsPath()
	if err != nil {
		return err
	}
	return writeFileAtomic(p, []byte(token+"\n"), 0o600)
}

func loadFileToken() (string, error) {
	p, err := config.CredentialsPath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errMissing
		}
		return "", err
	}
	tok := strings.TrimSpace(string(b))
	if tok == "" {
		return "", errMissing
	}
	return tok, nil
}

func clearFileToken() error {
	p, err := config.CredentialsPath()
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-credentials-*")
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
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	ok = true
	return nil
}
