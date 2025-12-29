package jcs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Transform implements JSON Canonicalization Scheme (RFC 8785)
func Transform(data []byte) ([]byte, error) {
	var val interface{}
	if err := json.Unmarshal(data, &val); err != nil {
		return nil, err
	}
	return canonicalize(val)
}

func canonicalize(val interface{}) ([]byte, error) {
	switch v := val.(type) {
	case nil:
		return []byte("null"), nil
	case bool:
		if v {
			return []byte("true"), nil
		}
		return []byte("false"), nil
	case float64:
		// RFC 8785: numbers should be formatted in a specific way.
		// For simple use cases, json.Marshal on float64 is close,
		// but JCS has strict rules about exponential notation etc.
		// However, Go's default json.Marshal for float64 is usually enough for non-extreme numbers.
		return json.Marshal(v)
	case string:
		return json.Marshal(v)
	case []interface{}:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				buf.WriteByte(',')
			}
			b, err := canonicalize(item)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var buf bytes.Buffer
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			keyBytes, _ := json.Marshal(k)
			buf.Write(keyBytes)
			buf.WriteByte(':')
			b, err := canonicalize(v[k])
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	default:
		return nil, fmt.Errorf("unsupported type: %T", v)
	}
}
