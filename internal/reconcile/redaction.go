package reconcile

import "strings"

const redactedValue = "[redacted]"

var sensitiveKeyMarkers = []string{
	"TOKEN",
	"SECRET",
	"PASSWORD",
	"PASSWD",
	"API_KEY",
	"ACCESS_KEY",
	"PRIVATE_KEY",
	"CREDENTIAL",
	"AUTHORIZATION",
}

func RedactSensitiveMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return values
	}
	result := make(map[string]string, len(values))
	for key, value := range values {
		if isSensitiveKey(key) {
			result[key] = redactedValue
			continue
		}
		result[key] = value
	}
	return result
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToUpper(key)
	normalized = strings.NewReplacer("-", "_", ".", "_", "/", "_").Replace(normalized)
	for _, marker := range sensitiveKeyMarkers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return strings.HasSuffix(normalized, "_KEY")
}
