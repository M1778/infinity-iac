package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	StatusRunning = "running"
	StatusStopped = "stopped"
	StatusDeleted = "deleted"
)

type AccessProfile struct {
	Name             string    `json:"name"`
	Provider         string    `json:"provider"`
	CredentialSource string    `json:"credential_source"`
	Account          string    `json:"account,omitempty"`
	Region           string    `json:"region,omitempty"`
	ValidatedAt      time.Time `json:"validated_at,omitempty"`
}

type ServerSpec struct {
	Name             string `json:"name,omitempty" yaml:"name,omitempty"`
	OS               string `json:"os,omitempty" yaml:"os,omitempty"`
	CPU              int    `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	RAMGB            int    `json:"ram_gb,omitempty" yaml:"ram_gb,omitempty"`
	StorageGB        int    `json:"storage_gb,omitempty" yaml:"storage_gb,omitempty"`
	Provider         string `json:"provider,omitempty" yaml:"provider,omitempty"`
	Access           string `json:"access,omitempty" yaml:"access,omitempty"`
	Region           string `json:"region,omitempty" yaml:"region,omitempty"`
	NetworkingScript string `json:"networking_script,omitempty" yaml:"networking_script,omitempty"`
	StartupScript    string `json:"startup_script,omitempty" yaml:"startup_script,omitempty"`
	Port             int    `json:"port,omitempty" yaml:"port,omitempty"`
}

func (s ServerSpec) ResolveDefaults(defaultProvider, defaultAccess, defaultRegion string) ServerSpec {
	if s.OS == "" {
		s.OS = "ubuntu-latest"
	}
	if s.CPU == 0 {
		s.CPU = 1
	}
	if s.RAMGB == 0 {
		s.RAMGB = 1
	}
	if s.StorageGB == 0 {
		s.StorageGB = 20
	}
	if s.Provider == "" {
		s.Provider = defaultProvider
	}
	if s.Access == "" {
		s.Access = defaultAccess
	}
	if s.Region == "" {
		s.Region = defaultRegion
	}
	if s.NetworkingScript == "" {
		s.NetworkingScript = "iauto"
	}
	return s
}

func (s ServerSpec) Validate() error {
	if s.Name == "" {
		return errors.New("server name is required")
	}
	if strings.TrimSpace(s.OS) == "" {
		return errors.New("os is required")
	}
	if s.CPU < 1 {
		return errors.New("cpu must be at least 1")
	}
	if s.RAMGB < 1 {
		return errors.New("ram must be at least 1GB")
	}
	if s.StorageGB < 1 {
		return errors.New("storage must be at least 1GB")
	}
	if s.Provider == "" {
		return errors.New("provider is required")
	}
	return nil
}

type Instance struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Provider  string            `json:"provider"`
	Access    string            `json:"access,omitempty"`
	Region    string            `json:"region,omitempty"`
	OS        string            `json:"os"`
	CPU       int               `json:"cpu"`
	RAMGB     int               `json:"ram_gb"`
	StorageGB int               `json:"storage_gb"`
	Status    string            `json:"status"`
	PublicIP  string            `json:"public_ip,omitempty"`
	Owner     string            `json:"owner"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Port      int               `json:"port,omitempty"`
}

type Operation struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type Manifest struct {
	APIVersion string       `json:"api_version" yaml:"api_version"`
	Project    string       `json:"project,omitempty" yaml:"project,omitempty"`
	Servers    []ServerSpec `json:"servers,omitempty" yaml:"servers,omitempty"`
}

type Action struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Resource string      `json:"resource"`
	Spec     *ServerSpec `json:"spec,omitempty"`
	Reason   string      `json:"reason,omitempty"`
}

type Plan struct {
	APIVersion string    `json:"api_version"`
	ID         string    `json:"id"`
	Actions    []Action  `json:"actions"`
	CreatedAt  time.Time `json:"created_at"`
	Hash       string    `json:"hash"`
}

func NewPlan(manifest Manifest, existing []Instance, now time.Time) (Plan, error) {
	if manifest.APIVersion == "" {
		manifest.APIVersion = "v1"
	}
	existingByName := make(map[string]Instance, len(existing))
	for _, in := range existing {
		existingByName[in.Name] = in
	}
	p := Plan{APIVersion: manifest.APIVersion, ID: fmt.Sprintf("plan-%d", now.UnixNano()), CreatedAt: now}
	for i := range manifest.Servers {
		s := manifest.Servers[i].ResolveDefaults(manifest.Servers[i].Provider, manifest.Servers[i].Access, manifest.Servers[i].Region)
		if err := s.Validate(); err != nil {
			return Plan{}, fmt.Errorf("servers[%d]: %w", i, err)
		}
		if old, ok := existingByName[s.Name]; ok {
			if old.Status == StatusDeleted {
				p.Actions = append(p.Actions, Action{ID: fmt.Sprintf("create-%s", s.Name), Type: "create", Resource: s.Name, Spec: &s, Reason: "deleted resource will be recreated"})
			}
			continue
		}
		p.Actions = append(p.Actions, Action{ID: fmt.Sprintf("create-%s", s.Name), Type: "create", Resource: s.Name, Spec: &s})
	}
	p.Hash = p.ComputeHash()
	return p, nil
}

func (p Plan) ComputeHash() string {
	p.Hash = ""
	b, _ := json.Marshal(struct {
		APIVersion string   `json:"api_version"`
		Actions    []Action `json:"actions"`
	}{p.APIVersion, p.Actions})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (p Plan) ValidHash() bool { return p.Hash != "" && p.Hash == p.ComputeHash() }
