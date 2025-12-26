package jcs

import (
	"bytes"
	"os"
	"testing"
)

func TestTransform(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "basic",
			input:    `{"b":"2","a":"1"}`,
			expected: `{"a":"1","b":"2"}`,
		},
		{
			name:     "nested",
			input:    `{"z":null,"a":{"b":2,"a":1}}`,
			expected: `{"a":{"a":1,"b":2},"z":null}`,
		},
		{
			name:     "array",
			input:    `{"b":[3,2,1],"a":true}`,
			expected: `{"a":true,"b":[3,2,1]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Transform([]byte(tt.input))
			if err != nil {
				t.Fatalf("Transform failed: %v", err)
			}
			if string(got) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(got))
			}
		})
	}
}

func TestGolden(t *testing.T) {
	input, err := os.ReadFile("../../testdata/jcs/input.json")
	if err != nil {
		t.Skip("Golden test input missing")
	}
	expected, err := os.ReadFile("../../testdata/jcs/expected.json")
	if err != nil {
		t.Skip("Golden test expected missing")
	}

	got, err := Transform(input)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	if !bytes.Equal(got, bytes.TrimSpace(expected)) {
		t.Errorf("Golden test failed")
	}
}
