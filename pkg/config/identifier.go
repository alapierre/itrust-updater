package config

import (
	"fmt"
	"strings"
)

// ValidateIdentifier accepts portable single-segment file names, including when
// a Windows-style name is supplied on Unix. IDs are never cleaned or rewritten.
func ValidateIdentifier(kind, value string) error {
	valid := value != "" && !strings.HasSuffix(value, ".")
	for i, c := range value {
		alphanumeric := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !alphanumeric && (i == 0 || c != '-' && c != '_' && c != '.') {
			valid = false
		}
	}
	stem, _, _ := strings.Cut(strings.ToUpper(value), ".")
	reserved := stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL"
	if len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9' {
		reserved = true
	}
	if !valid || reserved {
		return fmt.Errorf("invalid %s %q: use an ASCII letter or digit followed by letters, digits, '.', '_' or '-'; trailing dots and reserved device names are not allowed", kind, value)
	}
	return nil
}
