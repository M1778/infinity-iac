package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type catalogInput struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}

type catalogRecipe struct {
	Name          string         `json:"name"`
	Version       string         `json:"version"`
	Kind          string         `json:"kind"`
	Status        string         `json:"status"`
	Execution     string         `json:"execution"`
	Description   string         `json:"description"`
	Requirements  []string       `json:"requirements"`
	Providers     []string       `json:"providers"`
	OS            []string       `json:"os"`
	Architectures []string       `json:"architectures"`
	CPU           int            `json:"cpu"`
	RAMGB         int            `json:"ram_gb"`
	StorageGB     int            `json:"storage_gb"`
	Inputs        []catalogInput `json:"inputs"`
	Source        string         `json:"source"`
}

type catalog struct {
	Schema  string          `json:"schema"`
	Recipes []catalogRecipe `json:"recipes"`
}

type manifest struct {
	APIVersion   string   `yaml:"api_version"`
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version"`
	Kind         string   `yaml:"kind"`
	Status       string   `yaml:"status"`
	Execution    string   `yaml:"execution"`
	Description  string   `yaml:"description"`
	Requirements []string `yaml:"requirements"`
	Supports     struct {
		Providers     []string `yaml:"providers"`
		OS            []string `yaml:"os"`
		Architectures []string `yaml:"architectures"`
	} `yaml:"supports"`
	Resources struct {
		CPU       int `yaml:"cpu"`
		RAMGB     int `yaml:"ram_gb"`
		StorageGB int `yaml:"storage_gb"`
	} `yaml:"resources"`
	Inputs map[string]struct {
		Type     string `yaml:"type"`
		Required bool   `yaml:"required"`
	} `yaml:"inputs"`
}

var recipeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func loadCatalog(root string) (catalog, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return catalog{}, err
	}
	result := catalog{Schema: "docs-catalog/v1", Recipes: []catalogRecipe{}}
	seen := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name(), "recipe.yaml")
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return catalog{}, err
		}
		var item manifest
		if err := yaml.Unmarshal(data, &item); err != nil {
			return catalog{}, fmt.Errorf("decode %s: %w", path, err)
		}
		if item.APIVersion != "recipe/v1" || !recipeName.MatchString(item.Name) || strings.TrimSpace(item.Version) == "" {
			return catalog{}, fmt.Errorf("invalid recipe metadata: %s", path)
		}
		if seen[item.Name] {
			return catalog{}, fmt.Errorf("duplicate recipe name: %s", item.Name)
		}
		seen[item.Name] = true
		if item.Kind != "application" && item.Kind != "script" {
			return catalog{}, fmt.Errorf("invalid recipe kind: %s", path)
		}
		if item.Status == "" {
			item.Status = "unspecified"
		}
		if item.Status != "experimental" && item.Status != "template" && item.Status != "verified" && item.Status != "unspecified" {
			return catalog{}, fmt.Errorf("invalid recipe status: %s", path)
		}
		if item.Execution == "" {
			item.Execution = "instance"
		}
		if item.Execution != "instance" && item.Execution != "local" {
			return catalog{}, fmt.Errorf("invalid execution target: %s", path)
		}
		inputs := make([]catalogInput, 0, len(item.Inputs))
		for name, input := range item.Inputs {
			inputs = append(inputs, catalogInput{Name: name, Type: input.Type, Required: input.Required})
		}
		sort.Slice(inputs, func(i, j int) bool { return inputs[i].Name < inputs[j].Name })
		result.Recipes = append(result.Recipes, catalogRecipe{
			Name: item.Name, Version: item.Version, Kind: item.Kind, Status: item.Status,
			Execution: item.Execution, Description: item.Description, Requirements: nonNil(item.Requirements),
			Providers: nonNil(item.Supports.Providers), OS: nonNil(item.Supports.OS), Architectures: nonNil(item.Supports.Architectures),
			CPU: item.Resources.CPU, RAMGB: item.Resources.RAMGB, StorageGB: item.Resources.StorageGB,
			Inputs: inputs, Source: "https://github.com/M1778/infinity-iac/blob/master/recipes/" + entry.Name() + "/recipe.yaml",
		})
	}
	if len(result.Recipes) == 0 {
		return catalog{}, errors.New("no built-in recipe manifests found")
	}
	sort.Slice(result.Recipes, func(i, j int) bool { return result.Recipes[i].Name < result.Recipes[j].Name })
	return result, nil
}

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func searchText(item catalogRecipe) string {
	parts := []string{item.Name, item.Description, item.Kind, item.Status, item.Execution}
	parts = append(parts, item.Requirements...)
	parts = append(parts, item.Providers...)
	parts = append(parts, item.OS...)
	parts = append(parts, item.Architectures...)
	for _, input := range item.Inputs {
		parts = append(parts, input.Name)
	}
	return strings.ToLower(strings.Join(strings.Fields(strings.Join(parts, " ")), " "))
}

var entryTemplate = template.Must(template.New("entry").Funcs(template.FuncMap{
	"search": searchText,
	"join": func(values []string) string {
		if len(values) == 0 {
			return "Not declared"
		}
		return strings.Join(values, ", ")
	},
	"label": func(value string) string {
		if value == "" {
			return value
		}
		return strings.ToUpper(value[:1]) + value[1:]
	},
}).Parse(`{{range .Recipes}}
                        <details class="recipe-entry" id="recipe-{{.Name}}" data-kind="{{.Kind}}" data-status="{{.Status}}" data-search="{{search .}}">
                            <summary><span class="recipe-summary-main"><span class="recipe-title"><span class="recipe-name">{{.Name}}</span><span class="recipe-kind">{{.Kind}}{{if eq .Execution "local"}} · local{{end}}</span></span><span class="recipe-description">{{.Description}}</span></span><span class="recipe-summary-end"><span class="recipe-status">{{label .Status}}</span><svg class="recipe-chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m6 3 5 5-5 5" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></svg></span></summary>
                            <div class="recipe-body">
                                <dl class="recipe-meta"><div><dt>Definition version</dt><dd>{{.Version}}</dd></div><div><dt>Execution target</dt><dd>{{.Execution}}</dd></div><div><dt>Providers</dt><dd>{{join .Providers}}</dd></div><div><dt>Architectures declared</dt><dd>{{join .Architectures}}</dd></div><div><dt>Guest OS labels</dt><dd>{{join .OS}}</dd></div><div><dt>Requested resources</dt><dd>{{.CPU}} CPU · {{.RAMGB}} GB RAM · {{.StorageGB}} GB storage</dd></div></dl>
                                {{if .Requirements}}<div class="recipe-requirements"><h3 class="recipe-subheading">Requirements</h3><ul>{{range .Requirements}}<li>{{.}}</li>{{end}}</ul></div>{{end}}
                                {{if .Inputs}}<h3 class="recipe-subheading">Declared inputs</h3><ul class="recipe-inputs">{{range .Inputs}}<li><code>{{.Name}}</code> · {{.Type}}{{if .Required}} · required{{end}}</li>{{end}}</ul>{{end}}
                                <p class="recipe-note">{{if eq .Status "template"}}This definition is incomplete and cannot render or deploy. Complete the documented prerequisites before evaluating it.{{else}}Declared support and resource values are metadata, not runtime certification. Inspect the rendered script and test the selected platform.{{end}}</p>
                                <div class="recipe-links"><a href="{{.Source}}">Read manifest</a><a href="#recipe-{{.Name}}">Link to {{.Name}}</a></div>
                            </div>
                        </details>{{end}}
`))

func renderEntries(items catalog) ([]byte, error) {
	var output bytes.Buffer
	err := entryTemplate.Execute(&output, items)
	lines := strings.Split(output.String(), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return []byte(strings.Join(lines, "\n")), err
}

func replaceSection(page []byte, name string, content []byte) ([]byte, error) {
	start := []byte("<!-- " + name + ":start -->")
	end := []byte("<!-- " + name + ":end -->")
	if bytes.Count(page, start) != 1 || bytes.Count(page, end) != 1 {
		return nil, fmt.Errorf("expected one generated %s section", name)
	}
	startIndex := bytes.Index(page, start) + len(start)
	endIndex := bytes.Index(page, end)
	if endIndex < startIndex {
		return nil, fmt.Errorf("invalid generated %s section order", name)
	}
	result := append([]byte{}, page[:startIndex]...)
	result = append(result, '\n')
	result = append(result, content...)
	result = append(result, page[endIndex:]...)
	return result, nil
}

func renderSummary(items catalog) []byte {
	apps, scripts := 0, 0
	statuses := map[string]int{}
	for _, item := range items.Recipes {
		if item.Kind == "application" {
			apps++
		} else {
			scripts++
		}
		statuses[item.Status]++
	}
	parts := []string{}
	for _, status := range []string{"experimental", "template", "verified", "unspecified"} {
		if count := statuses[status]; count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, status))
		}
	}
	return []byte(fmt.Sprintf("                    <p class=\"catalog-summary\">%d definitions: %d applications and %d scripts. %s. Counts describe definitions, not successful deployments.</p>\n                    ", len(items.Recipes), apps, scripts, strings.Join(parts, "; ")))
}

func writeOrCheck(path string, data []byte, check bool) error {
	if check {
		current, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !bytes.Equal(current, data) {
			return fmt.Errorf("%s is stale; run go run ./cmd/docs-data", path)
		}
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("docs-data", flag.ContinueOnError)
	root := flags.String("recipes", "recipes", "built-in recipe directory")
	output := flags.String("out", "docs-site/data/catalog.json", "public catalog output")
	pagePath := flags.String("page", "docs-site/index.html", "documentation page")
	check := flags.Bool("check", false, "fail if generated public catalog is stale")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("docs-data accepts flags only")
	}
	items, err := loadCatalog(*root)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	entries, err := renderEntries(items)
	if err != nil {
		return err
	}
	page, err := os.ReadFile(*pagePath)
	if err != nil {
		return err
	}
	page, err = replaceSection(page, "catalog-summary", renderSummary(items))
	if err != nil {
		return err
	}
	page, err = replaceSection(page, "catalog-entries", entries)
	if err != nil {
		return err
	}
	if err := writeOrCheck(*output, data, *check); err != nil {
		return err
	}
	if err := writeOrCheck(*pagePath, page, *check); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "Documentation catalog: %d definitions\n", len(items.Recipes))
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "docs-data:", err)
		os.Exit(1)
	}
}
