package devcontainer

import (
	"fmt"
	"sort"
	"strings"
)

func resolveFeatureValueOptions(source string, raw any) (map[string]any, bool, error) {
	switch value := raw.(type) {
	case nil:
		return nil, true, nil
	case bool:
		return nil, value, nil
	case string:
		return map[string]any{"version": value}, true, nil
	case map[string]any:
		return value, true, nil
	default:
		return nil, false, fmt.Errorf("feature %q must be a boolean, string, object, or null", source)
	}
}

func materializeFeatureOptions(source string, manifest featureManifest, overrides map[string]any) (map[string]string, error) {
	result := map[string]string{}
	for key, option := range manifest.Options {
		if option.Default == nil {
			continue
		}
		value, err := stringifyFeatureOptionValue(source, key, option.Default)
		if err != nil {
			return nil, err
		}
		result[featureOptionEnvName(key)] = value
	}
	for key, value := range overrides {
		if _, ok := manifest.Options[key]; !ok {
			return nil, fmt.Errorf("feature %q does not declare option %q", source, key)
		}
		stringValue, err := stringifyFeatureOptionValue(source, key, value)
		if err != nil {
			return nil, err
		}
		result[featureOptionEnvName(key)] = stringValue
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

func stringifyFeatureOptionValue(source string, key string, value any) (string, error) {
	switch v := value.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case bool:
		if v {
			return "true", nil
		}
		return "false", nil
	case float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(v), nil
	default:
		return "", fmt.Errorf("feature %q option %q must be a scalar value", source, key)
	}
}

func featureOptionEnvName(key string) string {
	var b strings.Builder
	for i, r := range key {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_':
			if i == 0 && r >= '0' && r <= '9' {
				b.WriteByte('_')
			}
			if r >= 'a' && r <= 'z' {
				b.WriteRune(r - ('a' - 'A'))
			} else {
				b.WriteRune(r)
			}
		case r >= '0' && r <= '9':
			if i == 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "_"
	}
	return b.String()
}

func sortedKeys(values map[string]any) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
