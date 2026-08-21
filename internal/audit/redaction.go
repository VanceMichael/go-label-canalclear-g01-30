package audit

import (
	"strings"
)

var sensitiveKeys = map[string]struct{}{
	"authorization": {},
	"password":      {},
	"secret":        {},
	"session":       {},
	"token":         {},
}

func RedactDetails(details map[string]string) map[string]string {
	result := make(map[string]string, len(details))
	for key, value := range details {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if isSensitive(normalized) {
			result[key] = "[redacted]"
			continue
		}
		result[key] = strings.TrimSpace(value)
	}
	return result
}

func isSensitive(key string) bool {
	if _, ok := sensitiveKeys[key]; ok {
		return true
	}
	for candidate := range sensitiveKeys {
		if strings.HasSuffix(key, "_"+candidate) || strings.HasPrefix(key, candidate+"_") {
			return true
		}
	}
	return false
}
