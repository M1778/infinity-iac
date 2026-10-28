package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/M1778/infinity-iac/internal/config"
	"github.com/M1778/infinity-iac/internal/recipe"
	"gopkg.in/yaml.v3"
)

const maxAppManifestBytes = 2 << 20

var appNamePattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9._-]*$")

func (a *App) app(args []string) int {
	if len(args) == 0 || args[0] == "help" {
		fmt.Fprintln(a.out, "usage: app install OWNER/REPOSITORY [--yes] | app install --external HTTPS_URL [--yes] | app install PATH [--yes]")
		return 0
	}
	if args[0] != "install" {
		return a.fail("unknown app action " + args[0])
	}
	return a.appInstall(args[1:])
}

func (a *App) appInstall(args []string) int {
	if len(args) == 0 {
		return a.fail("app install requires OWNER/REPOSITORY, --external HTTPS_URL, or a local recipe path")
	}
	parseArgs := make([]string, 0, len(args))
	var source string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--external" || arg == "--output" {
			if i+1 >= len(args) {
				return a.fail(arg + " requires a value")
			}
			parseArgs = append(parseArgs, arg, args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(arg, "--external=") || strings.HasPrefix(arg, "-") {
			parseArgs = append(parseArgs, arg)
			continue
		}
		if source != "" {
			return a.fail("app install accepts one source")
		}
		source = arg
	}
	fs := flag.NewFlagSet("app install", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	external := fs.String("external", "", "HTTPS URL of an app manifest")
	yes := fs.Bool("yes", false, "confirm installation of third-party code")
	mode := fs.String("output", "table", "table or json")
	if err := fs.Parse(parseArgs); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return a.fail("app install accepts one source")
	}
	if *mode != "table" && *mode != "json" {
		return a.fail("output must be table or json")
	}
	if *external != "" && source != "" {
		return a.fail("choose one local/repository source or --external")
	}
	if *external != "" {
		r, err := loadRemoteRecipe(*external)
		if err != nil {
			return a.fail(err.Error())
		}
		return a.storeApp(r, "external URL", *yes, *mode)
	}
	if source == "" {
		return a.fail("app install requires a source")
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return a.fail("remote manifests require --external")
	}
	if _, statErr := os.Stat(source); statErr != nil && !filepath.IsAbs(source) && !strings.HasPrefix(source, ".") && strings.Contains(source, "/") {
		r, err := loadGitHubRecipe(source)
		if err != nil {
			return a.fail(err.Error())
		}
		return a.storeApp(r, "GitHub "+source, *yes, *mode)
	}
	r, err := recipe.Load(source)
	if err != nil {
		return a.fail(err.Error())
	}
	for _, step := range r.Steps {
		if step.Type == "upload" && !filepath.IsAbs(step.Source) {
			return a.fail(fmt.Sprintf("local recipe upload step %q must use an absolute source path; remote bundles with relative uploads are unsupported", step.ID))
		}
	}
	return a.storeApp(r, "local file", *yes, *mode)
}

func isGitHubRepo(value string) bool {
	parts := strings.Split(value, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.ContainsAny(value, "\\: ") && !strings.HasPrefix(value, ".")
}

func loadGitHubRecipe(repoName string) (recipe.Recipe, error) {
	parts := strings.Split(repoName, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(repoName, "\\: ") {
		return recipe.Recipe{}, errors.New("GitHub source must be OWNER/REPOSITORY")
	}
	for _, filename := range []string{"app.yml", "app.yaml", "recipe.yml", "recipe.yaml", "recipe.json"} {
		r, err := loadRemoteRecipe("https://raw.githubusercontent.com/" + parts[0] + "/" + parts[1] + "/HEAD/" + filename)
		if err == nil {
			return r, nil
		}
		if !strings.Contains(err.Error(), "HTTP 404") {
			return recipe.Recipe{}, err
		}
	}
	return recipe.Recipe{}, fmt.Errorf("GitHub repository %q has no app.yml, app.yaml, recipe.yml, recipe.yaml, or recipe.json", repoName)
}

func loadRemoteRecipe(rawURL string) (recipe.Recipe, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return recipe.Recipe{}, errors.New("external app manifest must use HTTPS")
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Get(u.String())
	if err != nil {
		return recipe.Recipe{}, fmt.Errorf("download app manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return recipe.Recipe{}, fmt.Errorf("download app manifest: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxAppManifestBytes {
		return recipe.Recipe{}, fmt.Errorf("app manifest exceeds %d bytes", maxAppManifestBytes)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxAppManifestBytes+1))
	if err != nil {
		return recipe.Recipe{}, fmt.Errorf("read app manifest: %w", err)
	}
	if len(b) > maxAppManifestBytes {
		return recipe.Recipe{}, fmt.Errorf("app manifest exceeds %d bytes", maxAppManifestBytes)
	}
	ext := filepath.Ext(u.Path)
	if ext != ".json" && ext != ".yaml" && ext != ".yml" {
		ext = ".yaml"
	}
	tmp, err := os.CreateTemp("", "infinity-iac-app-*"+ext)
	if err != nil {
		return recipe.Recipe{}, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0o600)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return recipe.Recipe{}, err
	}
	if err := tmp.Close(); err != nil {
		return recipe.Recipe{}, err
	}
	r, err := recipe.Load(name)
	if err != nil {
		return recipe.Recipe{}, err
	}
	for _, step := range r.Steps {
		if step.Type == "upload" {
			return recipe.Recipe{}, fmt.Errorf("remote recipe contains upload step %q; install a complete local bundle instead", step.ID)
		}
	}
	return r, nil
}

func (a *App) storeApp(r recipe.Recipe, source string, yes bool, mode string) int {
	if !appNamePattern.MatchString(r.Name) {
		return a.fail("recipe name must contain only letters, numbers, dot, underscore, or hyphen")
	}
	if !yes && !isTerminal(a.in) {
		return a.fail("app install requires --yes in noninteractive mode")
	}
	fmt.Fprintln(a.err, "WARNING: third-party recipes can execute arbitrary commands as root on OCI instances. Review the recipe before running it.")
	if !yes && !a.confirm("install third-party recipe "+r.Name) {
		return 1
	}
	items, _, err := loadRecipes("")
	if err != nil {
		return a.fail(err.Error())
	}
	for _, item := range items {
		if item.Name == r.Name {
			return a.fail(fmt.Sprintf("recipe %q is already installed or built in", r.Name))
		}
	}
	configPath, err := config.Path()
	if err != nil {
		return a.fail(err.Error())
	}
	dir := filepath.Join(config.AppsDir(configPath), r.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return a.fail("create app directory: " + err.Error())
	}
	assets := filepath.Join(dir, "assets")
	for i := range r.Steps {
		if r.Steps[i].Type != "upload" {
			continue
		}
		if !filepath.IsAbs(r.Steps[i].Source) {
			return a.fail(fmt.Sprintf("upload source %q for step %q must be absolute", r.Steps[i].Source, r.Steps[i].ID))
		}
		data, readErr := os.ReadFile(r.Steps[i].Source)
		if readErr != nil {
			return a.fail(fmt.Sprintf("read upload source %q for step %q: %v", r.Steps[i].Source, r.Steps[i].ID, readErr))
		}
		if err := os.MkdirAll(assets, 0o700); err != nil {
			return a.fail("create app assets: " + err.Error())
		}
		assetName := fmt.Sprintf("%02d-%s", i, filepath.Base(r.Steps[i].Source))
		assetPath := filepath.Join(assets, assetName)
		if err := os.WriteFile(assetPath, data, 0o600); err != nil {
			return a.fail("save app asset: " + err.Error())
		}
		r.Steps[i].Source = assetPath
	}
	b, err := yaml.Marshal(r)
	if err != nil {
		return a.fail("encode recipe: " + err.Error())
	}
	target := filepath.Join(dir, "recipe.yaml")
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return a.fail("save recipe: " + err.Error())
	}
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return a.fail("save recipe: " + err.Error())
	}
	result := map[string]any{"name": r.Name, "version": r.Version, "kind": r.Kind, "source": source, "path": target, "status": "installed"}
	if mode == "json" {
		return a.emit(result, nil)
	}
	fmt.Fprintf(a.out, "installed recipe %q at %s\n", r.Name, target)
	return 0
}
