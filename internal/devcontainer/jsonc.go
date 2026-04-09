package devcontainer

import "github.com/lauritsk/hatchctl/internal/spec"

func standardizeJSONC(path string, data []byte) ([]byte, error) {
	return spec.StandardizeJSONC(path, data)
}
