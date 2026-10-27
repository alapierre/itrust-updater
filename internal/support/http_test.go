package support

import (
	"testing"
	"time"

	"github.com/alapierre/itrust-updater/pkg/backend"
	"github.com/alapierre/itrust-updater/pkg/config"
)

func TestTransferTimeoutConfiguration(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want time.Duration
	}{
		{"", backend.DefaultTransferIdleTimeout}, {"30s", 30 * time.Second}, {"5m", 5 * time.Minute},
		{"0", 0}, {"-1s", 0}, {"broken", 0},
	} {
		b, err := NewNexusBackend("https://example.invalid", "", "", config.Config{"ITRUST_TRANSFER_IDLE_TIMEOUT": tc.raw})
		if tc.want == 0 {
			if err == nil {
				t.Fatalf("invalid timeout accepted: %q", tc.raw)
			}
		} else if err != nil || b.TransferIdleTimeout != tc.want {
			t.Fatalf("timeout %q: backend=%+v, err=%v", tc.raw, b, err)
		}
	}
}
