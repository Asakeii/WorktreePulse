package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type FileStore struct {
	dir string
}

func NewFileStore(dir string) *FileStore {
	return &FileStore{dir: dir}
}

func (s *FileStore) path() string {
	return filepath.Join(s.dir, "config.json")
}

func (s *FileStore) Load() (Config, error) {
	raw, err := os.ReadFile(s.path())
	if err != nil {
		if os.IsNotExist(err) {
			return normalizeConfig(Config{}), nil
		}
		return Config{}, err
	}
	return decodeConfig(raw)
}

func (s *FileStore) Save(cfg Config) (Config, error) {
	cfg = normalizeConfig(cfg)
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return Config{}, err
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Config{}, err
	}
	if err := os.WriteFile(s.path(), raw, 0o644); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
