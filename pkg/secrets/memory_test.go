package secrets

import "testing"

func TestInMemorySecretStore(t *testing.T) {
	ss := NewInMemorySecretStore()

	err := ss.Set("svc", "key", "val")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, err := ss.Get("svc", "key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if val != "val" {
		t.Errorf("Expected val, got %s", val)
	}

	err = ss.Delete("svc", "key")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = ss.Get("svc", "key")
	if err == nil {
		t.Errorf("Expected error for non-existent secret")
	}
}
