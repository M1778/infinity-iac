package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/M1778/infinity-iac/internal/recipe"
)

func (a *App) renderRecipe(args []string) int {
	fs := flag.NewFlagSet("recipe render", flag.ContinueOnError)
	fs.SetOutput(a.err)
	name := fs.String("name", "", "built-in or installed recipe name")
	file := fs.String("file", "", "recipe YAML or JSON file")
	port := fs.Int("port", 0, "override the service port")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if (*name == "") == (*file == "") || fs.NArg() != 0 {
		return a.fail("recipe render requires exactly one of --name or --file")
	}
	var item recipe.Recipe
	if *file != "" {
		var err error
		item, err = recipe.Load(*file)
		if err != nil {
			return a.fail(err.Error())
		}
	} else {
		var ok bool
		item, ok = findRecipe(*name)
		if !ok {
			return a.fail(fmt.Sprintf("recipe %q was not found; use list recipes", *name))
		}
	}
	if *port == 0 {
		*port = recipePort(item)
	}
	script, err := recipe.RenderCloudInit(item, *port)
	if err != nil {
		return a.fail("render recipe: " + err.Error())
	}
	if _, err := fmt.Fprint(a.out, strings.TrimSpace(script)+"\n"); err != nil {
		return a.fail(err.Error())
	}
	return 0
}
