package statemigrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixture struct {
	Name string `json:"name"`
}

func TestReadListMigratesLegacyAndBacksUp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	legacy := []byte(`[{"name":"one"}]`)
	if err := os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	items, err := ReadList[fixture](path, "items")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "one" {
		t.Fatalf("unexpected items: %#v", items)
	}
	backup, err := os.ReadFile(path + ".v0.bak")
	if err != nil || string(backup) != string(legacy) {
		t.Fatalf("backup = %q, %v", backup, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		SchemaVersion int       `json:"schema_version"`
		Items         []fixture `json:"items"`
	}
	if err = json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != CurrentVersion || len(doc.Items) != 1 {
		t.Fatalf("unexpected migrated document: %#v", doc)
	}
	if _, err = ReadList[fixture](path, "items"); err != nil {
		t.Fatalf("migration is not idempotent: %v", err)
	}
}

func TestReadListRejectsFutureVersionWithoutRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	original := []byte(`{"schema_version":99,"items":[]}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadList[fixture](path, "items")
	if err == nil || !strings.Contains(err.Error(), "newer than supported") {
		t.Fatalf("unexpected error: %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(original) {
		t.Fatal("future document was modified")
	}
	if _, err = os.Stat(path + ".v0.bak"); !os.IsNotExist(err) {
		t.Fatal("future document unexpectedly produced a backup")
	}
}

func TestReadRecordMigratesLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "record.json")
	if err := os.WriteFile(path, []byte(`{"name":"one"}`), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRecord[fixture](path)
	if err != nil || got.Name != "one" {
		t.Fatalf("record = %#v, %v", got, err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"schema_version": 1`) {
		t.Fatalf("record was not versioned: %s", data)
	}
}

func TestWriteListRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "items.json")
	if err := WriteList(path, "items", []fixture{{Name: "one"}}); err != nil {
		t.Fatal(err)
	}
	items, err := ReadList[fixture](path, "items")
	if err != nil || len(items) != 1 || items[0].Name != "one" {
		t.Fatalf("round trip = %#v, %v", items, err)
	}
}
