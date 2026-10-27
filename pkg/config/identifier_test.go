package config

import "testing"

func TestValidateIdentifier(t *testing.T) {
	for _, id := range []string{"main-repo", "go-ksef", "app_1", "Repo.2", "123", "x..y", "COM10"} {
		if err := ValidateIdentifier("ID", id); err != nil {
			t.Errorf("valid ID %q: %v", id, err)
		}
	}
	for _, id := range []string{"", ".", "..", "../other", `..\other`, "a/b", `a\b`, "/absolute", `C:\absolute`, `C:relative`, `\\server\share`, "a:b", "name.", "name ", "a\nb", "a\rb", "a\x00b", "%2e%2e", "CON", "con.txt", "LPT1", "COM9.log", "NUL", "żółw"} {
		if err := ValidateIdentifier("ID", id); err == nil {
			t.Errorf("unsafe ID %q accepted", id)
		}
	}
}
