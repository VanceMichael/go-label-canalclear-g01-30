package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const maximumStructuredDetailBytes = 64 * 1024

type DetailSanitizer struct {
	RedactedValue string
}

func DefaultDetailSanitizer() DetailSanitizer {
	return DetailSanitizer{RedactedValue: "[redacted]"}
}

func (sanitizer DetailSanitizer) Sanitize(details map[string]string) map[string]string {
	result := make(map[string]string, len(details))
	redactedValue := strings.TrimSpace(sanitizer.RedactedValue)
	if redactedValue == "" {
		redactedValue = "[redacted]"
	}
	for key, value := range details {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if isSensitive(normalizedKey) {
			result[key] = redactedValue
			continue
		}
		result[key] = sanitizer.sanitizeStructuredValue(value, redactedValue)
	}
	return result
}

func (sanitizer DetailSanitizer) sanitizeStructuredValue(value, redactedValue string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len(trimmed) > maximumStructuredDetailBytes {
		return trimmed
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return trimmed
	}
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return trimmed
	}
	if decoder.More() {
		return trimmed
	}
	redacted, changed := sanitizeTopLevelJSON(document, redactedValue)
	if !changed {
		return trimmed
	}
	encoded, err := encodeCompactJSON(redacted)
	if err != nil {
		return trimmed
	}
	return encoded
}

func sanitizeTopLevelJSON(document any, redactedValue string) (any, bool) {
	object, ok := document.(map[string]any)
	if !ok {
		return document, false
	}
	changed := false
	copyObject := make(map[string]any, len(object))
	for key, value := range object {
		normalizedKey := strings.ToLower(strings.TrimSpace(key))
		if isSensitive(normalizedKey) {
			copyObject[key] = redactedValue
			changed = true
			continue
		}
		copyObject[key] = value
	}
	return copyObject, changed
}

func encodeCompactJSON(document any) (string, error) {
	payload, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("marshal sanitized audit details: %w", err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, payload); err != nil {
		return "", fmt.Errorf("compact sanitized audit details: %w", err)
	}
	return compact.String(), nil
}

func SanitizeEvents(events []Event, sanitizer DetailSanitizer) []Event {
	result := make([]Event, len(events))
	for index, event := range events {
		result[index] = event
		result[index].Details = sanitizer.Sanitize(event.Details)
	}
	return result
}
