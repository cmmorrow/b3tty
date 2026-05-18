package src

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// LockFile holds the runtime state written when a b3tty server starts.
// It lets other b3tty subcommands locate the running server and construct
// authenticated URLs without the user having to supply port or token.
type LockFile struct {
	Port     int    `json:"port"`
	PID      int    `json:"pid"`
	Token    string `json:"token"`
	Protocol string `json:"protocol"`
}

func lockFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, DOT_CONFIG_PATH, B3TTY_CONFIG_PATH, LOCK_FILE_NAME), nil
}

// WriteLockFile creates or overwrites the lock file with the server's
// runtime values. The config directory is created if it does not exist.
func WriteLockFile(port, pid int, token, protocol string) error {
	path, err := lockFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(LockFile{Port: port, PID: pid, Token: token, Protocol: protocol})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// ReadLockFile reads and parses the lock file. It returns (nil, nil) when
// the file does not exist, indicating no server is running.
func ReadLockFile() (*LockFile, error) {
	path, err := lockFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var lf LockFile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, err
	}
	return &lf, nil
}

// RemoveLockFile deletes the lock file. It is a no-op if the file does
// not exist.
func RemoveLockFile() error {
	path, err := lockFilePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
