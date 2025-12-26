package sign

import (
	"encoding/base64"
	"testing"
)

func TestSignVerify(t *testing.T) {
	// Generated a random seed for testing
	seedB64 := base64.StdEncoding.EncodeToString(make([]byte, 32))
	payload := []byte("hello world")

	sig, err := Sign(payload, seedB64)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	pubKey, err := SeedToPubKey(seedB64)
	if err != nil {
		t.Fatalf("SeedToPubKey failed: %v", err)
	}

	err = Verify(payload, sig, pubKey)
	if err != nil {
		t.Errorf("Verify failed: %v", err)
	}

	// Test invalid signature
	err = Verify([]byte("corrupted"), sig, pubKey)
	if err == nil {
		t.Errorf("Verify should have failed for corrupted payload")
	}
}

func TestFingerprint(t *testing.T) {
	pubKey := []byte("some public key")
	expected := SHA256(pubKey)

	err := VerifyFingerprint(pubKey, expected)
	if err != nil {
		t.Errorf("VerifyFingerprint failed: %v", err)
	}

	err = VerifyFingerprint(pubKey, "wrong")
	if err == nil {
		t.Errorf("VerifyFingerprint should have failed for wrong fingerprint")
	}
}
