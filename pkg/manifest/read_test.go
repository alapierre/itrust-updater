package manifest

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

type endlessMetadata struct{ read int64 }

func (r *endlessMetadata) Read(p []byte) (int, error) {
	clear(p)
	r.read += int64(len(p))
	return len(p), nil
}

func TestReadManifestLimits(t *testing.T) {
	if _, err := Read(strings.NewReader("{}" + strings.Repeat(" ", int(MaxManifestSize)-2))); err != nil {
		t.Fatalf("document at limit rejected: %v", err)
	}
	r := &endlessMetadata{}
	if _, err := Read(r); err == nil || r.read != MaxManifestSize+1 {
		t.Fatalf("unbounded read: bytes=%d, err=%v", r.read, err)
	}
	if _, err := Read(strings.NewReader("{} {}")); err == nil {
		t.Fatal("second JSON document accepted")
	}
	if _, err := Read(iotest.ErrReader(io.ErrUnexpectedEOF)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("read failure lost: %v", err)
	}
}
