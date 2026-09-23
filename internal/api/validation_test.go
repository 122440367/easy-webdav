package api

import "testing"

func TestValidateRootDir(t *testing.T) {
	for _, value := range []string{"", ".", "..", "../other", "/absolute", `..\other`} {
		if _, err := ValidateRootDir(value); err == nil {
			t.Errorf("accepted unsafe root %q", value)
		}
	}
	if got, err := ValidateRootDir("team/alice"); err != nil || got != "team/alice" {
		t.Fatalf("valid root rejected: %q %v", got, err)
	}
}
func TestValidateUsername(t *testing.T) {
	if ValidateUsername("alice") != nil || ValidateUsername("../alice") == nil {
		t.Fatal("username validation failed")
	}
}
