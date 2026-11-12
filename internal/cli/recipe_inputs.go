package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/M1778/infinity-iac/internal/recipe"
	"gopkg.in/yaml.v3"
)

func loadRecipeInputs(path string) (map[string]any, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open recipe inputs: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, errors.New("recipe inputs must be a regular file of at most 1 MiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("recipe inputs exceed 1 MiB")
	}
	var values map[string]any
	if ext := strings.ToLower(filepath.Ext(path)); ext == ".yaml" || ext == ".yml" {
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&values); err != nil {
			return nil, errors.New("recipe inputs must be a YAML object")
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return nil, errors.New("recipe inputs must contain one document")
		}
	} else {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&values); err != nil {
			return nil, errors.New("recipe inputs must be a JSON object")
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return nil, errors.New("recipe inputs must contain one document")
		}
	}
	if values == nil {
		return nil, errors.New("recipe inputs must be an object, not null")
	}
	return values, nil
}

func recipeHasSensitiveInputs(item recipe.Recipe, values map[string]any) bool {
	for key, input := range item.Inputs {
		if input.Sensitive && values[key] != nil {
			return true
		}
	}
	return false
}
