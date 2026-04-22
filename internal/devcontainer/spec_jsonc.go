package devcontainer

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/tailscale/hujson"
)

var jsoncPositionPattern = regexp.MustCompile(`line\s+(\d+),\s*column\s+(\d+)`)

func StandardizeJSONC(path string, data []byte) ([]byte, error) {
	standardized, err := hujson.Standardize(data)
	if err != nil {
		return nil, formatJSONCError(path, data, err)
	}
	return standardized, nil
}

func formatJSONCError(path string, data []byte, err error) error {
	message := fmt.Sprintf("parse jsonc %s: %v", path, err)
	matches := jsoncPositionPattern.FindStringSubmatch(err.Error())
	if len(matches) != 3 {
		return fmt.Errorf("%s", message)
	}
	line, lineErr := strconv.Atoi(matches[1])
	column, columnErr := strconv.Atoi(matches[2])
	if lineErr != nil || columnErr != nil {
		return fmt.Errorf("%s", message)
	}
	lines := strings.Split(string(data), "\n")
	if line < 1 || line > len(lines) {
		return fmt.Errorf("%s", message)
	}
	contextLine := lines[line-1]
	pointer := strings.Repeat(" ", max(column-1, 0)) + "^"
	return fmt.Errorf("%s\n\n%d | %s\n    %s\nhint: check for a missing comma, extra trailing content, or mismatched braces near this location", message, line, contextLine, pointer)
}
