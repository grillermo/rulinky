package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDotenvPrecedence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, ".env", "# comment\n\nRULINKY_T_A=from_env\nRULINKY_T_B=\"quoted\"\nRULINKY_T_C=from_env\nRULINKY_T_REAL=from_file\nnot a pair\n")
	writeFile(t, dir, ".env.local", "RULINKY_T_C = from_local \n")
	t.Setenv("RULINKY_T_REAL", "from_process")
	for _, k := range []string{"RULINKY_T_A", "RULINKY_T_B", "RULINKY_T_C"} {
		t.Cleanup(func() { os.Unsetenv(k) })
	}

	if err := LoadDotenv(dir); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"RULINKY_T_A":    "from_env",
		"RULINKY_T_B":    "quoted",
		"RULINKY_T_C":    "from_local",
		"RULINKY_T_REAL": "from_process",
	}
	for k, v := range want {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestLoadDotenvWithoutFiles(t *testing.T) {
	if err := LoadDotenv(t.TempDir()); err != nil {
		t.Fatalf("missing files must be ignored, got %v", err)
	}
}
