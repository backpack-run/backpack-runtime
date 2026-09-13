package compute

import (
	"strings"
	"testing"
)

func TestRuntimeChildEnvironmentOmitsCloudAPIKey(t *testing.T) {
	environment := sanitizedChildEnvironment([]string{"PATH=test", "BACKPACK_API_KEY=cloud-secret", "backpack_api_key=case-insensitive-secret"})
	joined := strings.Join(environment, "\n")
	if joined != "PATH=test" {
		t.Fatalf("sensitive environment reached runtime child: %q", joined)
	}
}
