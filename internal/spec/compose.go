package spec

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
)

type MountSpec struct {
	Type            string
	Source          string
	Target          string
	ReadOnly        bool
	Consistency     string
	BindPropagation string
	CreateHostPath  *bool
	SELinux         string
	NoCopy          bool
	Subpath         string
	Raw             string
}

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
		result = "hatchctl"
	}
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(workspace))
	return fmt.Sprintf("%s_%x", result, hash.Sum64())
}

func ParseMountSpec(raw string) (MountSpec, bool) {
	parts := map[string]string{}
	for _, segment := range splitMountSegments(raw) {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		key, value, ok := strings.Cut(segment, "=")
		if !ok {
			parts[segment] = "true"
			continue
		}
		parts[strings.TrimSpace(key)] = normalizeMountValue(value)
	}
	target := firstNonEmptyString(parts["target"], parts["dst"])
	if target == "" {
		return MountSpec{}, false
	}
	spec := MountSpec{
		Type:            parts["type"],
		Source:          firstNonEmptyString(parts["source"], parts["src"]),
		Target:          target,
		ReadOnly:        parseMountBool(parts["readonly"]) || parseMountBool(parts["ro"]),
		Consistency:     parts["consistency"],
		BindPropagation: parts["bind-propagation"],
		SELinux:         parts["selinux"],
		NoCopy:          parseMountBool(parts["nocopy"]),
		Subpath:         parts["subpath"],
		Raw:             raw,
	}
	if value, ok := optionalMountBool(parts, "create-host-path"); ok {
		spec.CreateHostPath = value
	}
	return spec, true
}

func parseMountBool(value string) bool {
	return strings.EqualFold(value, "true") || value == "1"
}

func optionalMountBool(values map[string]string, key string) (*bool, bool) {
	value, ok := values[key]
	if !ok || value == "" {
		return nil, false
	}
	parsed := parseMountBool(value)
	return &parsed, true
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func splitMountSegments(raw string) []string {
	segments := make([]string, 0, strings.Count(raw, ",")+1)
	var current strings.Builder
	quote := rune(0)
	escaped := false
	for _, r := range raw {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			current.WriteRune(r)
			if r == quote {
				quote = 0
			}
		case r == '\'' || r == '"':
			quote = r
			current.WriteRune(r)
		case r == ',':
			segments = append(segments, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	segments = append(segments, current.String())
	return segments
}

func normalizeMountValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		if (value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"') {
			value = value[1 : len(value)-1]
		}
	}
	return value
}
