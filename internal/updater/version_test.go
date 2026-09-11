package updater

import "testing"

func TestSemanticVersionOrdering(t *testing.T) {
	ordered := []string{"v0.1.0-alpha.1", "0.1.0-alpha.2", "0.1.0-beta.1", "0.1.0", "0.2.0"}
	for i := 0; i < len(ordered)-1; i++ {
		left, leftErr := ParseVersion(ordered[i])
		right, rightErr := ParseVersion(ordered[i+1])
		if leftErr != nil || rightErr != nil || left.Compare(right) >= 0 {
			t.Fatalf("expected %q before %q: %v %v", ordered[i], ordered[i+1], leftErr, rightErr)
		}
	}
	if version, err := ParseVersion("0.1.0-alpha.1 (commit abc)"); err != nil || version.Major != 0 || version.Prerelease[0] != "alpha" {
		t.Fatalf("embedded build version parsing failed: %#v %v", version, err)
	}
	for _, invalid := range []string{"dev", "1.2", "01.2.3", "1.2.3-", "1.2.3-bad!"} {
		if _, err := ParseVersion(invalid); err == nil {
			t.Fatalf("invalid version accepted: %q", invalid)
		}
	}
}
