package main

import (
	"github.com/M1778/infinity-iac/internal/cli"
	"os"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }
