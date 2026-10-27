package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/M1778/infinity-iac/internal/domain"
	"os"
	"path/filepath"
)

type File struct {
	DefaultAccess string                 `json:"default_access,omitempty"`
	Profiles      []domain.AccessProfile `json:"profiles,omitempty"`
}

func Path() (string, error) {
	if p := os.Getenv("INFINITY_IAC_CONFIG"); p != "" {
		return p, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "infinity-iac", "config.json"), nil
}

func Load() (File, string, error) {
	p, err := Path()
	if err != nil {
		return File{}, "", err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return File{}, p, nil
	}
	if err != nil {
		return File{}, p, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		return File{}, p, fmt.Errorf("decode config: %w", err)
	}
	return f, p, nil
}

func Save(path string, f File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (f File) Profile(name string) (domain.AccessProfile, bool) {
	for _, p := range f.Profiles {
		if p.Name == name {
			return p, true
		}
	}
	return domain.AccessProfile{}, false
}

func (f File) DefaultProfile() (domain.AccessProfile, bool) {
	if f.DefaultAccess != "" {
		return f.Profile(f.DefaultAccess)
	}
	if len(f.Profiles) > 0 {
		return f.Profiles[0], true
	}
	return domain.AccessProfile{}, false
}

func (f *File) UpsertProfile(p domain.AccessProfile) {
	for i := range f.Profiles {
		if f.Profiles[i].Name == p.Name {
			f.Profiles[i] = p
			return
		}
	}
	f.Profiles = append(f.Profiles, p)
	if f.DefaultAccess == "" {
		f.DefaultAccess = p.Name
	}
}

func NewProfile(name, providerName, source, account, region string) domain.AccessProfile {
	return domain.AccessProfile{Name: name, Provider: providerName, CredentialSource: source, Account: account, Region: region}
}
