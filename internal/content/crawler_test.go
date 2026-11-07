package content

import (
	"os"
	"path/filepath"
	"testing"
)

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSyncProfileCopiesSessionFiles(t *testing.T) {
	real, auto := t.TempDir(), t.TempDir()
	mustWrite(t, filepath.Join(real, "Default", "Cookies"), "cookies")
	mustWrite(t, filepath.Join(real, "Default", "Local Storage", "leveldb", "000003.log"), "ls")
	mustWrite(t, filepath.Join(real, "Default", "IndexedDB", "site", "data"), "idb")
	mustWrite(t, filepath.Join(auto, "Default", "Local Storage", "stale"), "old")

	c := &ChromeCrawler{RealUserDataDir: real, UserDataDir: auto, Profile: "Default"}
	if err := c.syncProfile(); err != nil {
		t.Fatal(err)
	}

	for path, want := range map[string]string{
		"Cookies":                          "cookies",
		"Local Storage/leveldb/000003.log": "ls",
		"IndexedDB/site/data":              "idb",
	} {
		got, err := os.ReadFile(filepath.Join(auto, "Default", path))
		if err != nil || string(got) != want {
			t.Errorf("%s: got %q err %v", path, got, err)
		}
	}
	if _, err := os.Stat(filepath.Join(auto, "Default", "Local Storage", "stale")); !os.IsNotExist(err) {
		t.Error("stale file from the previous copy was kept")
	}
	if _, err := os.Stat(filepath.Join(auto, "Default", "Session Storage")); !os.IsNotExist(err) {
		t.Error("missing source must be skipped, not created")
	}
}
