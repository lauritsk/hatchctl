package devcontainer

import (
	"encoding/json"

	"github.com/tailscale/hujson"
)

func jsonMarshalIndent(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func standardizeJSONC(data []byte) ([]byte, error) {
	return hujson.Standardize(data)
}
