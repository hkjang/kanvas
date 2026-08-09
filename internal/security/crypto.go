package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Vault struct {
	aead cipher.AEAD
}

func OpenVault(dataDir string) (*Vault, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	path := filepath.Join(dataDir, "master.key")
	key, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return nil, fmt.Errorf("generate master key: %w", err)
		}
		if err = os.WriteFile(path, key, 0600); err != nil {
			return nil, fmt.Errorf("write master key: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	if len(key) != 32 {
		return nil, errors.New("invalid master key length")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Vault{aead: aead}, nil
}

func (v *Vault) Encrypt(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := v.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (v *Vault) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	b, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	n := v.aead.NonceSize()
	if len(b) < n {
		return "", errors.New("invalid encrypted value")
	}
	plain, err := v.aead.Open(nil, b[:n], b[n:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func RandomToken(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
