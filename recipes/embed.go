package recipes

import "embed"

// FS contains the built-in, versioned recipe catalog shipped with the CLI.
// External catalogs can use recipe.Discover for filesystem-backed development.
//
//go:embed n8n/recipe.yaml
var FS embed.FS
