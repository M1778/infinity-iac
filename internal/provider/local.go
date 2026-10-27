package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/M1778/infinity-iac/internal/domain"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type localState struct {
	Instances []domain.Instance `json:"instances"`
}

// FileProvider is deterministic and local. Its interface is the seam for real cloud adapters.
type FileProvider struct {
	path string
	mu   sync.Mutex
	now  func() time.Time
}

func NewFileProvider(path string) *FileProvider { return &FileProvider{path: path, now: time.Now} }
func (p *FileProvider) Name() string            { return "local" }

func (p *FileProvider) load() (localState, error) {
	b, err := os.ReadFile(p.path)
	if errors.Is(err, os.ErrNotExist) {
		return localState{}, nil
	}
	if err != nil {
		return localState{}, err
	}
	var s localState
	if err := json.Unmarshal(b, &s); err != nil {
		return localState{}, fmt.Errorf("decode provider state: %w", err)
	}
	return s, nil
}
func (p *FileProvider) save(s localState) error {
	if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.path)
}
func match(i domain.Instance, f Filter) bool {
	return (f.Provider == "" || i.Provider == f.Provider) && (f.Access == "" || i.Access == f.Access) && (f.Region == "" || i.Region == f.Region)
}
func (p *FileProvider) ListInstances(ctx context.Context, f Filter) ([]domain.Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.load()
	if err != nil {
		return nil, err
	}
	result := make([]domain.Instance, 0, len(s.Instances))
	for _, i := range s.Instances {
		if match(i, f) && i.Status != domain.StatusDeleted {
			result = append(result, i)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
func (p *FileProvider) CreateInstance(ctx context.Context, req CreateRequest) (domain.Instance, error) {
	if err := ctx.Err(); err != nil {
		return domain.Instance{}, err
	}
	if err := req.Spec.Validate(); err != nil {
		return domain.Instance{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.load()
	if err != nil {
		return domain.Instance{}, err
	}
	for _, i := range s.Instances {
		if req.IdempotencyKey != "" && i.Metadata["idempotency_key"] == req.IdempotencyKey && i.Status != domain.StatusDeleted {
			return i, nil
		}
		if i.Name == req.Spec.Name && i.Status != domain.StatusDeleted {
			return domain.Instance{}, fmt.Errorf("instance %q already exists", i.Name)
		}
	}
	now := p.now().UTC()
	h := sha256.Sum256([]byte(req.Spec.Name + "\x00" + req.IdempotencyKey + "\x00" + now.Format(time.RFC3339Nano)))
	id := "inst-" + hex.EncodeToString(h[:])[:12]
	instance := domain.Instance{ID: id, Name: req.Spec.Name, Provider: req.Spec.Provider, Access: req.Spec.Access, Region: req.Spec.Region, OS: req.Spec.OS, CPU: req.Spec.CPU, RAMGB: req.Spec.RAMGB, StorageGB: req.Spec.StorageGB, Port: req.Spec.Port, Status: domain.StatusRunning, PublicIP: "192.0.2." + fmt.Sprint((len(s.Instances)%200)+1), Owner: "infinity-iac", Metadata: map[string]string{"backend": "file-provider", "idempotency_key": req.IdempotencyKey}, CreatedAt: now, UpdatedAt: now}
	s.Instances = append(s.Instances, instance)
	if err := p.save(s); err != nil {
		return domain.Instance{}, err
	}
	return instance, nil
}
func (p *FileProvider) update(ctx context.Context, id, status string) (domain.Instance, error) {
	if err := ctx.Err(); err != nil {
		return domain.Instance{}, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.load()
	if err != nil {
		return domain.Instance{}, err
	}
	for n := range s.Instances {
		if s.Instances[n].ID == id {
			if s.Instances[n].Status == domain.StatusDeleted {
				return domain.Instance{}, fmt.Errorf("instance %q is deleted", id)
			}
			s.Instances[n].Status = status
			s.Instances[n].UpdatedAt = p.now().UTC()
			if err := p.save(s); err != nil {
				return domain.Instance{}, err
			}
			return s.Instances[n], nil
		}
	}
	return domain.Instance{}, fmt.Errorf("instance %q not found", id)
}
func (p *FileProvider) StopInstance(ctx context.Context, id string) (domain.Instance, error) {
	return p.update(ctx, id, domain.StatusStopped)
}
func (p *FileProvider) RestartInstance(ctx context.Context, id string) (domain.Instance, error) {
	return p.update(ctx, id, domain.StatusRunning)
}
func (p *FileProvider) DeleteInstance(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	s, err := p.load()
	if err != nil {
		return err
	}
	for n := range s.Instances {
		if s.Instances[n].ID == id {
			s.Instances[n].Status = domain.StatusDeleted
			s.Instances[n].UpdatedAt = p.now().UTC()
			return p.save(s)
		}
	}
	return fmt.Errorf("instance %q not found", id)
}
