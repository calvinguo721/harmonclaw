// Package governor (crypto) provides Hasher, Encryptor, Signer interfaces.
package governor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"
)

// CryptoSuite 等保切换点：future "gm" 启用 SM3/SM4/SM2 国密套件
const CryptoSuite = "standard"

// Hasher 哈希接口。国密替换点：实现 SM3。
type Hasher interface {
	Hash(data []byte) []byte
}

// Encryptor 对称加密接口。国密替换点：实现 SM4-GCM。
type Encryptor interface {
	Encrypt(plaintext, key []byte) (ciphertext []byte, err error)
	Decrypt(ciphertext, key []byte) (plaintext []byte, err error)
}

// Signer 数字签名接口。国密替换点：实现 SM2。
type Signer interface {
	Sign(data []byte, privKey []byte) (sig []byte, err error)
	Verify(data, sig, pubKey []byte) bool
}

// --- standard 实现 ---

type stdHasher struct{}

func (h *stdHasher) Hash(data []byte) []byte {
	// SHA-256，国密替换为 SM3
	out := sha256Sum(data)
	return out[:]
}

type stdEncryptor struct{}

func (e *stdEncryptor) Encrypt(plaintext, key []byte) ([]byte, error) {
	return aesGCMEncrypt(plaintext, key)
}

func (e *stdEncryptor) Decrypt(ciphertext, key []byte) ([]byte, error) {
	return aesGCMDecrypt(ciphertext, key)
}

type stdSigner struct{}

func (s *stdSigner) Sign(data []byte, privKey []byte) ([]byte, error) {
	return ed25519Sign(data, privKey)
}

func (s *stdSigner) Verify(data, sig, pubKey []byte) bool {
	return ed25519Verify(data, sig, pubKey)
}

// StandardHasher 返回标准 Hasher 实现
func StandardHasher() Hasher { return &stdHasher{} }

// StandardEncryptor 返回标准 Encryptor 实现
func StandardEncryptor() Encryptor { return &stdEncryptor{} }

// StandardSigner 返回标准 Signer 实现
func StandardSigner() Signer { return &stdSigner{} }

func sha256Sum(data []byte) [32]byte {
	return sha256.Sum256(data)
}

func aesGCMEncrypt(plaintext, key []byte) ([]byte, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, errors.New("aes key must be 16/24/32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func aesGCMDecrypt(ciphertext, key []byte) ([]byte, error) {
	if len(key) != 16 && len(key) != 24 && len(key) != 32 {
		return nil, errors.New("aes key must be 16/24/32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, ciphertext[:ns], ciphertext[ns:], nil)
}

func ed25519Sign(data, privKey []byte) ([]byte, error) {
	if len(privKey) != ed25519.PrivateKeySize {
		return nil, errors.New("ed25519 privkey must be 64 bytes")
	}
	return ed25519.Sign(privKey, data), nil
}

func ed25519Verify(data, sig, pubKey []byte) bool {
	if len(pubKey) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pubKey, data, sig)
}
