package cli

import (
	"flag"
	"fmt"
	"strings"
)

func flagsBeforePositionals(fs *flag.FlagSet, args []string) ([]string, error) {
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		f := fs.Lookup(name)
		if f == nil {
			return nil, fmt.Errorf("unknown flag --%s", name)
		}
		flags = append(flags, arg)
		boolean, ok := f.Value.(interface{ IsBoolFlag() bool })
		if hasValue || ok && boolean.IsBoolFlag() {
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("--%s requires a value", name)
		}
		i++
		flags = append(flags, args[i])
	}
	return append(flags, positional...), nil
}
