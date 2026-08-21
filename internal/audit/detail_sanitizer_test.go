package audit

import (
	"strings"
	"testing"
)

func TestAuditSanitizerRedactsNestedStructuredSecrets(t *testing.T) {
	details := map[string]string{
		"request": `{"route":"customs.submit","credentials":{"access_token":"live-token","session":"session-42"},"attempts":[{"authorization":"Bearer live-token"}]}`,
		"status":  "rejected",
	}

	sanitized := DefaultDetailSanitizer().Sanitize(details)
	request := sanitized["request"]
	for _, secret := range []string{"live-token", "session-42", "Bearer"} {
		if strings.Contains(request, secret) {
			t.Fatalf("nested secret %q remains in sanitized request: %s", secret, request)
		}
	}
	if strings.Count(request, "[redacted]") != 3 {
		t.Fatalf("sanitized request=%s, want all three nested credentials redacted", request)
	}
	if sanitized["status"] != "rejected" {
		t.Fatalf("non-sensitive status changed: %q", sanitized["status"])
	}
	if strings.Contains(details["request"], "[redacted]") {
		t.Fatal("sanitizer mutated original audit details")
	}
}
