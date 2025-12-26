package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	input := `
# Comment
KEY1=VALUE1
  KEY2 = VALUE2  
INVALID LINE
`
	r := strings.NewReader(input)
	cfg, err := Parse(r)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	expected := Config{
		"KEY1": "VALUE1",
		"KEY2": "VALUE2",
	}

	if !reflect.DeepEqual(cfg, expected) {
		t.Errorf("Expected %v, got %v", expected, cfg)
	}
}

func TestMergeConfigs(t *testing.T) {
	c1 := Config{"A": "1", "B": "1"}
	c2 := Config{"B": "2", "C": "2"}
	c3 := Config{"C": "3", "D": "3"}

	res := MergeConfigs(c1, c2, c3)

	expected := Config{
		"A": "1",
		"B": "1", // c1 has highest priority
		"C": "2", // c2 has middle priority
		"D": "3", // c3 has lowest priority
	}

	if !reflect.DeepEqual(res, expected) {
		t.Errorf("Expected %v, got %v", expected, res)
	}
}
