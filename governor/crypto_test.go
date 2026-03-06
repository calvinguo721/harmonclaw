package governor

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	rand.Read(key)
	plain := []byte("sensitive data")
	ct, err := EncryptForStorage(plain, "sensitive", key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ct) <= len(plain) {
		t.Error("ciphertext should be longer than plaintext")
	}
	out, err := DecryptFromStorage(ct, "sensitive", key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(plain) {
		t.Errorf("decrypt: want %q, got %q", plain, out)
	}
}

func TestSignVerify(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	data := []byte("data to sign")
	sig, err := SignData(data, priv)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifySignature(data, sig, pub) {
		t.Error("valid signature should verify")
	}
	if VerifySignature(data, sig, pub[:1]) {
		t.Error("short pubkey should not verify")
	}
	if VerifySignature([]byte("tampered"), sig, pub) {
		t.Error("tampered data should not verify")
	}
}

func TestEncryptForStorage_NonSensitive(t *testing.T) {
	plain := []byte("public")
	out, err := EncryptForStorage(plain, "public", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(plain) {
		t.Errorf("public should pass through: got %q", out)
	}
}
