package secrets

import "testing"

func TestCipherRoundTrip(t *testing.T) {
	cipher, err := NewCipher("01234567890123456789012345678901")
	if err != nil {
		t.Fatal(err)
	}
	one, err := cipher.Seal("provider-key")
	if err != nil {
		t.Fatal(err)
	}
	two, err := cipher.Seal("provider-key")
	if err != nil {
		t.Fatal(err)
	}
	if one == two {
		t.Fatal("Seal() reused ciphertext; want random nonce")
	}
	got, err := cipher.Open(one)
	if err != nil {
		t.Fatal(err)
	}
	if got != "provider-key" {
		t.Fatalf("Open() = %q, want provider-key", got)
	}
}

func TestCipherRejectsShortKey(t *testing.T) {
	if _, err := NewCipher("short"); err == nil {
		t.Fatal("NewCipher() error = nil, want short-key error")
	}
}
