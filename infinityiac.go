package infinityiac

import (
	"time"

	"github.com/M1778/infinity-iac/internal/domain"
	"github.com/M1778/infinity-iac/internal/provider"
	"github.com/M1778/infinity-iac/internal/recipe"
	builtin "github.com/M1778/infinity-iac/recipes"
)

type AccessProfile = domain.AccessProfile
type ServerSpec = domain.ServerSpec
type Instance = domain.Instance
type Manifest = domain.Manifest
type Plan = domain.Plan
type Action = domain.Action
type NetworkSpec = domain.NetworkSpec
type Network = domain.Network
type CreateRequest = provider.CreateRequest
type NetworkCreateRequest = provider.NetworkCreateRequest
type Filter = provider.Filter
type Provider = provider.Provider
type NetworkProvider = provider.NetworkProvider
type OCIProvider = provider.OCIProvider
type OCINetworkProvider = provider.OCINetworkProvider
type Recipe = recipe.Recipe
type Catalog = recipe.Catalog
type RenderOptions = recipe.RenderOptions
type Input = recipe.Input
type Step = recipe.Step
type Supports = recipe.Supports
type Resources = recipe.Resources
type HealthCheck = recipe.HealthCheck
type Output = recipe.Output

const (
	StatusRunning      = domain.StatusRunning
	StatusStopped      = domain.StatusStopped
	StatusDeleted      = domain.StatusDeleted
	StatusProvisioning = domain.StatusProvisioning
	RecipeAPIVersion   = recipe.APIVersion
)

func NewOCIProvider(profile AccessProfile) (*OCIProvider, error) {
	return provider.NewOCIProvider(profile)
}

func NewOCINetworkProvider(profile AccessProfile) (*OCINetworkProvider, error) {
	return provider.NewOCINetworkProvider(profile)
}

func BuiltinCatalog() (Catalog, error) {
	items, err := recipe.DiscoverFS(builtin.FS)
	if err != nil {
		return Catalog{}, err
	}
	return recipe.NewCatalog(items)
}

func LoadRecipe(path string) (Recipe, error) {
	return recipe.Load(path)
}

func RenderCloudInit(item Recipe, options RenderOptions) (string, error) {
	return recipe.RenderCloudInitWithInputs(item, options)
}

func RenderLocalScript(item Recipe, options RenderOptions) (string, error) {
	return recipe.RenderLocalScript(item, options)
}

func ResolveInputs(item Recipe, values map[string]any) (map[string]string, error) {
	return recipe.ResolveInputs(item, values)
}

func NewPlan(manifest Manifest, existing []Instance, now time.Time) (Plan, error) {
	return domain.NewPlan(manifest, existing, now)
}
