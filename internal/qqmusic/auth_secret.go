package qqmusic

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

const (
	sessionFilePrefix = "lilt-encrypted-session-v1:"
	sessionFileKeyLen = 32
	maxSessionSize    = 64 << 10
)

type sessionSecrets interface {
	Get(string) ([]byte, error)
	Set(string, []byte) error
	Delete(string) error
}

var sessionSecretDir = func() (string, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "qqmusic-session.d"), nil
}

func sessionFileKey(entry []byte) ([]byte, error) {
	raw, ok := bytes.CutPrefix(entry, []byte(sessionFilePrefix))
	if !ok {
		return nil, nil
	}
	if len(raw) != sessionFileKeyLen {
		return nil, errors.New("qqmusic: invalid encrypted session reference")
	}
	return raw, nil
}

func sessionFilePath(key []byte) (string, error) {
	dir, err := sessionSecretDir()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(key)
	return filepath.Join(dir, hex.EncodeToString(digest[:])), nil
}

func sessionFileCipher(key []byte) cipher.AEAD {
	block, _ := aes.NewCipher(key)
	aead, _ := cipher.NewGCMWithRandomNonce(block)
	return aead
}

func readSessionSecret(store sessionSecrets) ([]byte, error) {
	entry, err := store.Get(qqSessionSecret)
	if err != nil {
		return nil, err
	}
	key, err := sessionFileKey(entry)
	if err != nil || key == nil {
		return entry, err
	}
	path, err := sessionFilePath(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("qqmusic: encrypted session file is missing")
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	aead := sessionFileCipher(key)
	limit := int64(maxSessionSize + aead.Overhead())
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errSessionTooLarge
	}
	return aead.Open(nil, nil, b, nil)
}

func writeSessionSecret(store sessionSecrets, b []byte) error {
	if len(b) > maxSessionSize {
		return errSessionTooLarge
	}
	prior, err := store.Get(qqSessionSecret)
	if err != nil && !errors.Is(err, mygo.ErrSecretNotFound) {
		return err
	}
	entry := b
	var path string
	if len(b) > mygo.MaxSecretSize {
		key := make([]byte, sessionFileKeyLen)
		rand.Read(key)
		path, err = sessionFilePath(key)
		if err != nil {
			return err
		}
		if err = writeSessionFile(path, sessionFileCipher(key).Seal(nil, nil, b, nil)); err != nil {
			return err
		}
		entry = append([]byte(sessionFilePrefix), key...)
	}
	// Publish the new key only after the ciphertext is durable.
	if err = store.Set(qqSessionSecret, entry); err != nil {
		if path != "" {
			os.Remove(path)
		}
		return err
	}
	removeSessionFile(prior)
	return nil
}

func writeSessionFile(path string, ciphertext []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(ciphertext); err == nil {
		err = f.Sync()
	}
	if err = errors.Join(err, f.Close()); err != nil {
		os.Remove(path)
	}
	return err
}

func deleteSessionSecret(store sessionSecrets) error {
	entry, _ := store.Get(qqSessionSecret)
	if err := store.Delete(qqSessionSecret); err != nil {
		return err
	}
	removeSessionFile(entry)
	return nil
}

func removeSessionFile(entry []byte) {
	key, err := sessionFileKey(entry)
	if err == nil && key != nil {
		if path, err := sessionFilePath(key); err == nil {
			os.Remove(path)
		}
	}
}
