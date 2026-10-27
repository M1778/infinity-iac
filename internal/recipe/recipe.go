package recipe

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const APIVersion = "recipe/v1"

type Recipe struct {
	APIVersion  string            `json:"api_version" yaml:"api_version"`
	Name        string            `json:"name" yaml:"name"`
	Version     string            `json:"version" yaml:"version"`
	Kind        string            `json:"kind" yaml:"kind"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Supports    Supports          `json:"supports" yaml:"supports"`
	Inputs      map[string]Input  `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Resources   Resources         `json:"resources" yaml:"resources"`
	Steps       []Step            `json:"steps" yaml:"steps"`
	Health      []HealthCheck     `json:"health,omitempty" yaml:"health,omitempty"`
	Outputs     map[string]Output `json:"outputs,omitempty" yaml:"outputs,omitempty"`
}

type Supports struct {
	Providers     []string `json:"providers,omitempty" yaml:"providers,omitempty"`
	OS            []string `json:"os,omitempty" yaml:"os,omitempty"`
	Architectures []string `json:"architectures,omitempty" yaml:"architectures,omitempty"`
}
type Input struct {
	Type        string `json:"type" yaml:"type"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	Default     any    `json:"default,omitempty" yaml:"default,omitempty"`
	Required    bool   `json:"required,omitempty" yaml:"required,omitempty"`
	Min         int    `json:"min,omitempty" yaml:"min,omitempty"`
	Max         int    `json:"max,omitempty" yaml:"max,omitempty"`
}
type Resources struct {
	Specs     string        `json:"specs,omitempty" yaml:"specs,omitempty"`
	CPU       int           `json:"cpu,omitempty" yaml:"cpu,omitempty"`
	RAMGB     int           `json:"ram_gb,omitempty" yaml:"ram_gb,omitempty"`
	StorageGB int           `json:"storage_gb,omitempty" yaml:"storage_gb,omitempty"`
	Ingress   []IngressRule `json:"ingress,omitempty" yaml:"ingress,omitempty"`
}
type IngressRule struct {
	Port     int    `json:"port" yaml:"port"`
	Protocol string `json:"protocol" yaml:"protocol"`
	Source   string `json:"source" yaml:"source"`
}
type Step struct {
	ID             string `json:"id" yaml:"id"`
	Type           string `json:"type" yaml:"type"`
	Command        string `json:"command,omitempty" yaml:"command,omitempty"`
	Source         string `json:"source,omitempty" yaml:"source,omitempty"`
	Destination    string `json:"destination,omitempty" yaml:"destination,omitempty"`
	Package        string `json:"package,omitempty" yaml:"package,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
}
type HealthCheck struct {
	Type           string `json:"type" yaml:"type"`
	Port           int    `json:"port,omitempty" yaml:"port,omitempty"`
	Path           string `json:"path,omitempty" yaml:"path,omitempty"`
	Command        string `json:"command,omitempty" yaml:"command,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
}
type Output struct {
	Type      string `json:"type" yaml:"type"`
	Value     string `json:"value,omitempty" yaml:"value,omitempty"`
	Sensitive bool   `json:"sensitive,omitempty" yaml:"sensitive,omitempty"`
}

func Load(path string) (Recipe, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Recipe{}, err
	}
	return decode(path, b)
}

func decode(path string, b []byte) (Recipe, error) {
	var r Recipe
	var err error
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		err = json.Unmarshal(b, &r)
	} else {
		err = yaml.Unmarshal(b, &r)
	}
	if err != nil {
		return Recipe{}, fmt.Errorf("decode recipe %s: %w", path, err)
	}
	if err := r.Validate(); err != nil {
		return Recipe{}, fmt.Errorf("validate recipe %s: %w", path, err)
	}
	return r, nil
}

func (r Recipe) Validate() error {
	if r.APIVersion != APIVersion {
		return fmt.Errorf("api_version must be %q", APIVersion)
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(r.Version) == "" {
		return errors.New("version is required")
	}
	if r.Kind != "application" && r.Kind != "script" {
		return errors.New("kind must be application or script")
	}
	for field, values := range map[string][]string{"supports.providers": r.Supports.Providers, "supports.os": r.Supports.OS, "supports.architectures": r.Supports.Architectures} {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				return fmt.Errorf("%s cannot contain empty values", field)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("%s contains duplicate value %q", field, value)
			}
			seen[value] = struct{}{}
		}
	}
	for key, in := range r.Inputs {
		if strings.TrimSpace(key) == "" {
			return errors.New("input name cannot be empty")
		}
		if in.Type != "string" && in.Type != "integer" && in.Type != "boolean" {
			return fmt.Errorf("input %q has unsupported type %q", key, in.Type)
		}
		if in.Min != 0 && in.Max != 0 && in.Min > in.Max {
			return fmt.Errorf("input %q has min greater than max", key)
		}
		if in.Min < 0 || in.Max < 0 {
			return fmt.Errorf("input %q bounds cannot be negative", key)
		}
		if (in.Min != 0 || in.Max != 0) && in.Type != "integer" {
			return fmt.Errorf("input %q bounds require integer type", key)
		}
		if in.Default != nil && !validInputDefault(in.Type, in.Default) {
			return fmt.Errorf("input %q default does not match type %q", key, in.Type)
		}
	}
	for i, rule := range r.Resources.Ingress {
		if rule.Port < 1 || rule.Port > 65535 {
			return fmt.Errorf("ingress[%d] port must be between 1 and 65535", i)
		}
		if rule.Protocol != "tcp" && rule.Protocol != "udp" {
			return fmt.Errorf("ingress[%d] protocol must be tcp or udp", i)
		}
		if strings.TrimSpace(rule.Source) == "" {
			return fmt.Errorf("ingress[%d] source is required", i)
		}
	}
	if r.Resources.CPU < 0 || r.Resources.RAMGB < 0 || r.Resources.StorageGB < 0 {
		return errors.New("resource CPU, RAM, and storage cannot be negative")
	}
	if len(r.Steps) == 0 {
		return errors.New("at least one step is required")
	}
	seen := make(map[string]struct{}, len(r.Steps))
	for i, step := range r.Steps {
		if strings.TrimSpace(step.ID) == "" {
			return fmt.Errorf("steps[%d] id is required", i)
		}
		if _, ok := seen[step.ID]; ok {
			return fmt.Errorf("duplicate step id %q", step.ID)
		}
		seen[step.ID] = struct{}{}
		switch step.Type {
		case "command":
			if strings.TrimSpace(step.Command) == "" {
				return fmt.Errorf("step %q command is required", step.ID)
			}
		case "upload":
			if step.Source == "" || step.Destination == "" {
				return fmt.Errorf("step %q upload requires source and destination", step.ID)
			}
		case "package":
			if step.Package == "" {
				return fmt.Errorf("step %q package is required", step.ID)
			}
		default:
			return fmt.Errorf("step %q has unsupported type %q", step.ID, step.Type)
		}
		if step.TimeoutSeconds < 0 {
			return fmt.Errorf("step %q timeout cannot be negative", step.ID)
		}
	}
	for i, check := range r.Health {
		if check.Type != "http" && check.Type != "command" {
			return fmt.Errorf("health[%d] type must be http or command", i)
		}
		if check.Port < 0 || check.Port > 65535 {
			return fmt.Errorf("health[%d] port is invalid", i)
		}
		if check.Type == "http" && check.Port == 0 {
			return fmt.Errorf("health[%d] http port is required", i)
		}
		if check.Type == "http" && strings.TrimSpace(check.Path) == "" {
			return fmt.Errorf("health[%d] http path is required", i)
		}
		if check.Type == "command" && strings.TrimSpace(check.Command) == "" {
			return fmt.Errorf("health[%d] command is required", i)
		}
		if check.TimeoutSeconds < 0 {
			return fmt.Errorf("health[%d] timeout cannot be negative", i)
		}
	}
	for name, output := range r.Outputs {
		if strings.TrimSpace(name) == "" {
			return errors.New("output name cannot be empty")
		}
		if output.Type != "string" && output.Type != "integer" && output.Type != "boolean" && output.Type != "secret" {
			return fmt.Errorf("output %q has unsupported type %q", name, output.Type)
		}
	}
	return nil
}

func validInputDefault(kind string, value any) bool {
	switch kind {
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		switch number := value.(type) {
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			return true
		case float32:
			return math.Trunc(float64(number)) == float64(number)
		case float64:
			return math.Trunc(number) == number
		}
	}
	return false
}

type StepExecution struct {
	Step   Step   `json:"step"`
	Status string `json:"status"`
}
type ExecutionPlan struct {
	Recipe  string          `json:"recipe"`
	Version string          `json:"version"`
	Steps   []StepExecution `json:"steps"`
}

func (r Recipe) Plan() ExecutionPlan {
	steps := make([]StepExecution, 0, len(r.Steps))
	for _, step := range r.Steps {
		steps = append(steps, StepExecution{Step: step, Status: "pending"})
	}
	return ExecutionPlan{Recipe: r.Name, Version: r.Version, Steps: steps}
}

func Discover(root string) ([]Recipe, error) {
	var paths []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		base := strings.ToLower(filepath.Base(path))
		if base == "recipe.yaml" || base == "recipe.yml" || base == "recipe.json" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	result := make([]Recipe, 0, len(paths))
	for _, path := range paths {
		r, err := Load(path)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}

// DiscoverFS loads recipe files from an embedded or otherwise read-only fs.FS.
// Paths are sorted before decoding so catalog output is deterministic.
func DiscoverFS(fsys fs.FS) ([]Recipe, error) {
	var paths []string
	err := fs.WalkDir(fsys, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		base := strings.ToLower(pathpkg.Base(path))
		if base == "recipe.yaml" || base == "recipe.yml" || base == "recipe.json" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	result := make([]Recipe, 0, len(paths))
	for _, path := range paths {
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, err
		}
		r, err := decode(path, b)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, nil
}
