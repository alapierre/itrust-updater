package sign

import (
	"encoding/base64"
	"strings"
	"testing"
)

type endlessKey struct{ read int }

func (r *endlessKey) Read(p []byte) (int, error) {
	clear(p)
	r.read += len(p)
	return len(p), nil
}

func TestReadPublicKeyLimits(t *testing.T) {
	for _, size := range []int{0, 31, 32, 33} {
		key, err := ReadPublicKey(strings.NewReader(strings.Repeat("k", size)))
		if (err == nil) != (size == 32) {
			t.Fatalf("size=%d, err=%v", size, err)
		}
		if err == nil && len(key) != size {
			t.Fatalf("unexpected key length: %d", len(key))
		}
	}
	r := &endlessKey{}
	if _, err := ReadPublicKey(r); err == nil || r.read != 33 {
		t.Fatalf("unbounded read: bytes=%d, err=%v", r.read, err)
	}
}

func TestVerifyInvalidPublicKeyLength(t *testing.T) {
	sig := base64.StdEncoding.EncodeToString(make([]byte, 64))
	for _, size := range []int{0, 31, 33} {
		if err := Verify([]byte("payload"), sig, make([]byte, size)); err == nil {
			t.Fatalf("accepted key size %d", size)
		}
	}
}
