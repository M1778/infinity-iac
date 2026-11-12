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
	inputsFile := fs.String("inputs", "", "JSON or YAML file containing typed input values")
	includeSensitive := fs.Bool("include-sensitive", false, "allow supplied sensitive values in rendered output")
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
	values, err := loadRecipeInputs(*inputsFile)
	if err != nil {
		return a.fail(err.Error())
	}
	if recipeHasSensitiveInputs(item, values) && !*includeSensitive {
		return a.fail("rendering supplied sensitive inputs requires --include-sensitive; protect the resulting file")
	}
	if *port == 0 && values["port"] == nil {
		*port = recipePort(item)
	}
	options := recipe.RenderOptions{Port: *port, Inputs: values}
	var script string
	if item.Execution == "local" {
		script, err = recipe.RenderLocalScript(item, options)
	} else {
		script, err = recipe.RenderCloudInitWithInputs(item, options)
	}
	if err != nil {
		return a.fail("render recipe: " + err.Error())
	}
	if _, err := fmt.Fprint(a.out, strings.TrimSpace(script)+"\n"); err != nil {
		return a.fail(err.Error())
	}
	return 0
}
