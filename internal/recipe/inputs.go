package recipe

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type RenderOptions struct {
	Port   int
	Inputs map[string]any
}

func inputEnvironment(name string, input Input) string {
	if input.Environment != "" {
		return input.Environment
	}
	return "INFINITY_INPUT_" + strings.ToUpper(name)
}

func ResolveInputs(item Recipe, supplied map[string]any) (map[string]string, error) {
	if err := item.Validate(); err != nil {
		return nil, err
	}
	for key := range supplied {
		if _, ok := item.Inputs[key]; !ok {
			return nil, fmt.Errorf("unknown input %q for recipe %q", key, item.Name)
		}
	}
	result := make(map[string]string, len(item.Inputs))
	for key, input := range item.Inputs {
		value, exists := supplied[key]
		if !exists || value == nil {
			value = input.Default
		}
		if value == nil {
			if input.Required {
				return nil, fmt.Errorf("required input %q is missing for recipe %q", key, item.Name)
			}
			switch input.Type {
			case "integer":
				result[key] = "0"
			case "boolean":
				result[key] = "false"
			default:
				result[key] = ""
			}
			continue
		}
		normalized, err := normalizeInput(key, input, value)
		if err != nil {
			return nil, err
		}
		if input.Required && input.Type == "string" && strings.TrimSpace(normalized) == "" {
			return nil, fmt.Errorf("required input %q cannot be empty", key)
		}
		result[key] = normalized
	}
	return result, nil
}

func normalizeInput(name string, input Input, value any) (string, error) {
	invalid := func() (string, error) {
		return "", fmt.Errorf("input %q must have type %s", name, input.Type)
	}
	switch input.Type {
	case "string":
		text, ok := value.(string)
		if !ok {
			return invalid()
		}
		if strings.ContainsRune(text, 0) || len(text) > 128<<10 {
			return "", fmt.Errorf("input %q exceeds the text limit or contains a NUL byte", name)
		}
		return text, nil
	case "boolean":
		boolean, ok := value.(bool)
		if !ok {
			return invalid()
		}
		return strconv.FormatBool(boolean), nil
	case "integer":
		var integer int64
		switch number := value.(type) {
		case int:
			integer = int64(number)
		case int8:
			integer = int64(number)
		case int16:
			integer = int64(number)
		case int32:
			integer = int64(number)
		case int64:
			integer = number
		case uint:
			if uint64(number) > math.MaxInt64 {
				return invalid()
			}
			integer = int64(number)
		case uint8:
			integer = int64(number)
		case uint16:
			integer = int64(number)
		case uint32:
			integer = int64(number)
		case uint64:
			if number > math.MaxInt64 {
				return invalid()
			}
			integer = int64(number)
		case json.Number:
			var err error
			integer, err = number.Int64()
			if err != nil {
				return invalid()
			}
		case float64:
			if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number < -9223372036854775808.0 || number >= 9223372036854775808.0 {
				return invalid()
			}
			integer = int64(number)
		case float32:
			return normalizeInput(name, input, float64(number))
		default:
			return invalid()
		}
		if input.Min != 0 && integer < int64(input.Min) || input.Max != 0 && integer > int64(input.Max) {
			return "", fmt.Errorf("input %q is outside its declared range", name)
		}
		return strconv.FormatInt(integer, 10), nil
	default:
		return invalid()
	}
}

func (item Recipe) CheckSupport(providerName, osName, architecture string) error {
	if err := item.Validate(); err != nil {
		return err
	}
	if providerName == "oci" {
		providerName = "oracle"
	}
	for _, check := range []struct {
		field  string
		value  string
		values []string
	}{{"provider", providerName, item.Supports.Providers}, {"OS", osName, item.Supports.OS}, {"architecture", architecture, item.Supports.Architectures}} {
		if len(check.values) == 0 {
			continue
		}
		found := false
		for _, allowed := range check.values {
			if allowed == check.value || check.field == "provider" && allowed == "oci" && check.value == "oracle" {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("recipe %q does not support %s %q", item.Name, check.field, check.value)
		}
	}
	return nil
}
