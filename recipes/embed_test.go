package recipes

import (
	"testing"

	"github.com/M1778/infinity-iac/internal/recipe"
)

func TestEmbeddedCatalogIsValid(t *testing.T) {
	items, err := recipe.DiscoverFS(FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("embedded catalog is empty")
	}
}
