package resume

import (
	"encoding/json"
	"os"
	"path/filepath"

	"cliamp/internal/appdir"
	"cliamp/internal/session"
)

func filePath(name string) (string, error) {
	dir, err := appdir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

func stateFile() (string, error) {
	return filePath("resume.json")
}

func lockFile() (string, error) {
	return filePath("resume.lock")
}

func Save(snapshot session.PersistedSnapshot) {
	snapshot = session.NormalizePersistedSnapshot(snapshot)
	if snapshot.Empty() {
		Clear()
		return
	}

	f, err := stateFile()
	if err != nil {
		return
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	if err := withLock(func() error {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			return err
		}
		return os.WriteFile(f, data, 0o600)
	}); err != nil {
		return
	}
}

func Load() session.PersistedSnapshot {
	f, err := stateFile()
	if err != nil {
		return session.PersistedSnapshot{}
	}

	var snapshot session.PersistedSnapshot
	if err := withLock(func() error {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &snapshot)
	}); err != nil {
		return session.PersistedSnapshot{}
	}
	return session.NormalizePersistedSnapshot(snapshot)
}

func Clear() {
	f, err := stateFile()
	if err != nil {
		return
	}
	_ = withLock(func() error {
		return os.Remove(f)
	})
}
