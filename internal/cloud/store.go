package cloud

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/backpack-run/backpack-runtime/internal/config"
)

var deviceIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Credentials struct {
	DeviceKeyID string
	PrivateKey  ed25519.PrivateKey
	Storage     string
}

type credentialMetadata struct {
	SchemaVersion int       `json:"schema_version"`
	DeviceKeyID   string    `json:"device_key_id"`
	Storage       string    `json:"storage"`
	CreatedAt     time.Time `json:"created_at"`
}

type CredentialStore struct{ directory string }

func NewCredentialStore(paths config.Paths) *CredentialStore {
	return &CredentialStore{directory: filepath.Join(paths.Config, "cloud")}
}

func (s *CredentialStore) metadataPath() string { return filepath.Join(s.directory, "device.json") }
func (s *CredentialStore) keyPath() string      { return filepath.Join(s.directory, "device.key") }

func (s *CredentialStore) Save(credentials Credentials) error {
	if !deviceIDPattern.MatchString(credentials.DeviceKeyID) {
		return fmt.Errorf("invalid Backpack Cloud device key ID")
	}
	if len(credentials.PrivateKey) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid Ed25519 private key length")
	}
	if err := os.MkdirAll(s.directory, 0700); err != nil {
		return err
	}
	protected, storage, err := protectSecret(credentials.PrivateKey)
	if err != nil {
		return err
	}
	defer zero(protected)
	metadata, err := json.MarshalIndent(credentialMetadata{SchemaVersion: 1, DeviceKeyID: credentials.DeviceKeyID, Storage: storage, CreatedAt: time.Now().UTC()}, "", "  ")
	if err != nil {
		return err
	}
	if err = writeAtomic(s.keyPath(), protected, 0600); err != nil {
		return err
	}
	if err = writeAtomic(s.metadataPath(), metadata, 0600); err != nil {
		_ = os.Remove(s.keyPath())
		return err
	}
	return nil
}

func (s *CredentialStore) Load() (Credentials, error) {
	var credentials Credentials
	metadataBytes, err := readSecureFile(s.metadataPath())
	if err != nil {
		return credentials, err
	}
	var metadata credentialMetadata
	if err = json.Unmarshal(metadataBytes, &metadata); err != nil {
		return credentials, fmt.Errorf("parse cloud credential metadata: %w", err)
	}
	if metadata.SchemaVersion != 1 || !deviceIDPattern.MatchString(metadata.DeviceKeyID) {
		return credentials, fmt.Errorf("cloud credential metadata is invalid or unsupported")
	}
	protected, err := readSecureFile(s.keyPath())
	if err != nil {
		return credentials, err
	}
	privateKey, err := unprotectSecret(protected, metadata.Storage)
	zero(protected)
	if err != nil {
		return credentials, fmt.Errorf("stored Backpack Cloud credential cannot be opened for this user or machine; run `backpack logout` and then `backpack login`: %w", err)
	}
	if len(privateKey) != ed25519.PrivateKeySize {
		zero(privateKey)
		return credentials, fmt.Errorf("stored cloud credential has an invalid key length")
	}
	return Credentials{DeviceKeyID: metadata.DeviceKeyID, PrivateKey: ed25519.PrivateKey(privateKey), Storage: metadata.Storage}, nil
}

func (s *CredentialStore) Delete() error {
	var result error
	for _, path := range []string{s.keyPath(), s.metadataPath()} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			result = errors.Join(result, err)
		}
	}
	return result
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".cloud-credential-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(mode); err == nil {
		_, err = temporary.Write(data)
	}
	if syncErr := temporary.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to replace symlinked credential file")
	}
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func readSecureFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("cloud credential path is not a regular file")
	}
	if err = validateSecretPermissions(info); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
