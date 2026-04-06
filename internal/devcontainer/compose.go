package devcontainer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveComposeFiles(configDir string, raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	var values []string
	switch v := raw.(type) {
	case string:
		values = []string{v}
	case []any:
		for _, item := range v {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("dockerComposeFile entries must be strings")
			}
			values = append(values, text)
		}
	default:
		return nil, fmt.Errorf("dockerComposeFile must be a string or array")
	}
	if len(values) == 0 {
		return nil, nil
	}
	resolved := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("dockerComposeFile entries cannot be empty")
		}
		path := value
		if !filepath.IsAbs(path) {
			path = filepath.Join(configDir, path)
		}
		path, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("compose file %q not found", value)
		}
		resolved = append(resolved, path)
	}
	return resolved, nil
}

func ComposeProjectName(workspace string, configPath string) string {
	base := filepath.Base(workspace)
	if filepath.Base(filepath.Dir(configPath)) == ".devcontainer" {
		base += "_devcontainer"
	}
	var b strings.Builder
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	result := strings.Trim(b.String(), "_")
	if result == "" {
		return "hatchctl"
	}
	return result
}
