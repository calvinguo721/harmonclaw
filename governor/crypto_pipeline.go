// Package governor (crypto_pipeline) auto-encrypts sensitive/secret data before write.
package governor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
)

const (
	ClassSensitive = "sensitive"
	ClassSecret    = "secret"
)

var encryptKey []byte

// SetEncryptKey sets the key for sensitive/secret encryption. Must be 16/24/32 bytes.
func SetEncryptKey(key []byte) error {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return errors.New("encrypt key must be 16/24/32 bytes")
	}
	encryptKey = make([]byte, len(key))
	copy(encryptKey, key)
	return nil
}

// GetEncryptKeyFromEnv reads key from HC_ENCRYPT_KEY (base64). Call at startup.
func GetEncryptKeyFromEnv() []byte {
	b64 := os.Getenv("HC_ENCRYPT_KEY")
	if b64 == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil
	}
	return key
}

// EncryptIfSensitive encrypts data when classification is sensitive or secret.
func EncryptIfSensitive(plaintext []byte, classification string) ([]byte, bool, error) {
	if classification != ClassSensitive && classification != ClassSecret {
		return plaintext, false, nil
	}
	key := encryptKey
	if len(key) == 0 {
		key = GetEncryptKeyFromEnv()
	}
	if len(key) == 0 {
		return plaintext, false, nil
	}
	enc := StandardEncryptor()
	ciphertext, err := enc.Encrypt(plaintext, key)
	if err != nil {
		return nil, false, err
	}
	out, _ := json.Marshal(map[string]string{
		"enc": "aes-gcm",
		"ct":  base64.StdEncoding.EncodeToString(ciphertext),
	})
	return out, true, nil
}

// DecryptIfEncrypted decrypts when data has enc wrapper.
func DecryptIfEncrypted(data []byte) ([]byte, bool, error) {
	var wrapper struct {
		Enc string `json:"enc"`
		Ct  string `json:"ct"`
	}
	if json.Unmarshal(data, &wrapper) != nil || wrapper.Enc == "" {
		return data, false, nil
	}
	key := encryptKey
	if len(key) == 0 {
		key = GetEncryptKeyFromEnv()
	}
	if len(key) == 0 {
		return data, false, errors.New("no encrypt key for decryption")
	}
	ct, err := base64.StdEncoding.DecodeString(wrapper.Ct)
	if err != nil {
		return nil, false, err
	}
	dec := StandardEncryptor()
	plain, err := dec.Decrypt(ct, key)
	if err != nil {
		return nil, false, err
	}
	return plain, true, nil
}
