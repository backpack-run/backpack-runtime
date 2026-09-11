// Package statemigrate provides the small, deliberately data-only migration
// layer used by Backpack's persisted JSON stores.
package statemigrate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const CurrentVersion = 1

// ReadList reads a versioned list document. The v0 format was a bare JSON
// array; it is migrated in place to {"schema_version":1,"<field>":[...]}.
func ReadList[T any](path, field string) ([]T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("decode legacy state %s: %w", path, err)
		}
		updated, err := MarshalList(field, items)
		if err != nil {
			return nil, err
		}
		if err := migrate(path, data, updated, 0); err != nil {
			return nil, err
		}
		return items, nil
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("decode state %s: %w", path, err)
	}
	version, err := versionOf(path, envelope)
	if err != nil {
		return nil, err
	}
	if version != CurrentVersion {
		return nil, fmt.Errorf("state %s schema_version %d is unsupported", path, version)
	}
	raw, ok := envelope[field]
	if !ok {
		return nil, fmt.Errorf("state %s is missing %q", path, field)
	}
	var items []T
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decode state %s field %q: %w", path, field, err)
	}
	return items, nil
}

// ReadRecord reads a versioned record. Legacy records were JSON objects with
// no schema_version and are migrated without changing their data fields.
func ReadRecord[T any](path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return zero, fmt.Errorf("decode state %s: %w", path, err)
	}
	rawVersion, hasVersion := envelope["schema_version"]
	if !hasVersion {
		var record T
		if err := json.Unmarshal(data, &record); err != nil {
			return zero, fmt.Errorf("decode legacy state %s: %w", path, err)
		}
		updated, err := MarshalRecord(record)
		if err != nil {
			return zero, err
		}
		if err := migrate(path, data, updated, 0); err != nil {
			return zero, err
		}
		return record, nil
	}
	var version int
	if err := json.Unmarshal(rawVersion, &version); err != nil {
		return zero, fmt.Errorf("state %s has invalid schema_version: %w", path, err)
	}
	if version > CurrentVersion {
		return zero, fmt.Errorf("state %s schema_version %d is newer than supported %d; upgrade Backpack", path, version, CurrentVersion)
	}
	if version != CurrentVersion {
		return zero, fmt.Errorf("state %s schema_version %d is unsupported", path, version)
	}
	delete(envelope, "schema_version")
	payload, err := json.Marshal(envelope)
	if err != nil {
		return zero, err
	}
	var record T
	if err := json.Unmarshal(payload, &record); err != nil {
		return zero, fmt.Errorf("decode state %s: %w", path, err)
	}
	return record, nil
}

func versionOf(path string, envelope map[string]json.RawMessage) (int, error) {
	raw, ok := envelope["schema_version"]
	if !ok {
		return 0, fmt.Errorf("state %s has no schema_version and is not a recognized legacy document", path)
	}
	var version int
	if err := json.Unmarshal(raw, &version); err != nil {
		return 0, fmt.Errorf("state %s has invalid schema_version: %w", path, err)
	}
	if version > CurrentVersion {
		return 0, fmt.Errorf("state %s schema_version %d is newer than supported %d; upgrade Backpack", path, version, CurrentVersion)
	}
	return version, nil
}

func MarshalList[T any](field string, items []T) ([]byte, error) {
	doc := map[string]any{"schema_version": CurrentVersion, field: items}
	return json.MarshalIndent(doc, "", "  ")
}

func MarshalRecord[T any](record T) ([]byte, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("state record must encode as a JSON object: %w", err)
	}
	version, _ := json.Marshal(CurrentVersion)
	doc["schema_version"] = version
	return json.MarshalIndent(doc, "", "  ")
}

// WriteList writes a current-version document using same-directory staging.
func WriteList[T any](path, field string, items []T) error {
	data, err := MarshalList(field, items)
	if err != nil {
		return err
	}
	return atomicReplace(path, data)
}

// WriteRecord writes a current-version record using same-directory staging.
func WriteRecord[T any](path string, record T) error {
	data, err := MarshalRecord(record)
	if err != nil {
		return err
	}
	return atomicReplace(path, data)
}

func migrate(path string, original, updated []byte, from int) error {
	backup := fmt.Sprintf("%s.v%d.bak", path, from)
	if prior, err := os.ReadFile(backup); err == nil {
		if !bytes.Equal(prior, original) {
			return fmt.Errorf("refuse to replace migration backup %s with different data", backup)
		}
	} else if os.IsNotExist(err) {
		if err := writeExclusive(backup, original); err != nil {
			return fmt.Errorf("back up state before migration: %w", err)
		}
	} else {
		return fmt.Errorf("read migration backup: %w", err)
	}
	if err := atomicReplace(path, updated); err != nil {
		return fmt.Errorf("migrate state %s: %w", path, err)
	}
	return nil
}

func writeExclusive(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(path)
		return err
	}
	return closeErr
}

func atomicReplace(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if _, err = os.Stat(path); os.IsNotExist(err) {
		return os.Rename(tmpPath, path)
	} else if err != nil {
		return err
	}
	old, err := os.CreateTemp(dir, ".state-replaced-*")
	if err != nil {
		return err
	}
	oldPath := old.Name()
	if err = old.Close(); err != nil {
		return err
	}
	if err = os.Remove(oldPath); err != nil {
		return err
	}
	if err = os.Rename(path, oldPath); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, path); err != nil {
		_ = os.Rename(oldPath, path)
		return err
	}
	_ = os.Remove(oldPath)
	return nil
}
