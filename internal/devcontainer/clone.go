package devcontainer

import (
	"maps"
	"slices"
)

func cloneSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return slices.Clone(values)
}

func cloneMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return maps.Clone(values)
}
