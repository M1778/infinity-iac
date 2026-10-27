package recipe

import (
	"fmt"
	"sort"
)

// Catalog provides deterministic lookup over a validated recipe set. Names
// must be unique so a command cannot silently select a different recipe.
type Catalog struct {
	items map[string]Recipe
}

func NewCatalog(items []Recipe) (Catalog, error) {
	c := Catalog{items: make(map[string]Recipe, len(items))}
	for _, item := range items {
		if err := item.Validate(); err != nil {
			return Catalog{}, fmt.Errorf("recipe %q: %w", item.Name, err)
		}
		if _, exists := c.items[item.Name]; exists {
			return Catalog{}, fmt.Errorf("duplicate recipe name %q", item.Name)
		}
		c.items[item.Name] = item
	}
	return c, nil
}

func (c Catalog) Get(name string) (Recipe, bool) {
	r, ok := c.items[name]
	return r, ok
}

func (c Catalog) List() []Recipe {
	result := make([]Recipe, 0, len(c.items))
	for _, item := range c.items {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
