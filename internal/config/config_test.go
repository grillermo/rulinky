package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func clearEnv(t *testing.T) {
	for _, k := range []string{"DATABASE_URL", "DATABASE_HOST", "DATABASE_PORT", "DATABASE_USER",
		"DATABASE_PASSWORD", "DATABASE_NAME", "SESSION_SECRET", "PORT", "RULINKY_ENV",
		"READITSOON_EMAIL", "SLACK_OTP_WEBHOOK_URL", "RULINKY_DISABLE_WORKER",
		"CHROME_REAL_USER_DATA_DIR", "CHROME_USER_DATA_DIR", "CHROME_PROFILE_DIRECTORY"} {
		t.Setenv(k, "")
	}
}

func TestFromEnvDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_HOST", "192.168.1.1")
	t.Setenv("DATABASE_USER", "grillermo")
	t.Setenv("DATABASE_PASSWORD", `it's\x`)
	t.Setenv("SESSION_SECRET", strings.Repeat("a", 64))

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}

	wantDSN := `host='192.168.1.1' port='5432' user='grillermo' password='it\'s\\x' dbname='rulinky'`
	if cfg.DatabaseURL != wantDSN {
		t.Errorf("DatabaseURL = %s, want %s", cfg.DatabaseURL, wantDSN)
	}
	if cfg.Port != "3002" {
		t.Errorf("Port = %q", cfg.Port)
	}
	if cfg.Production || cfg.DisableWorker {
		t.Errorf("Production/DisableWorker should default to false")
	}
	if cfg.ReaditsoonEmail != "guillermo.siliceo@kindle.com" {
		t.Errorf("ReaditsoonEmail = %q", cfg.ReaditsoonEmail)
	}
	home, _ := os.UserHomeDir()
	if cfg.ChromeRealUserDataDir != filepath.Join(home, "Library/Application Support/Google/Chrome") ||
		cfg.ChromeUserDataDir != filepath.Join(home, "Library/Application Support/Google/ChromeRulinky") ||
		cfg.ChromeProfileDirectory != "Default" {
		t.Errorf("chrome defaults wrong: %+v", cfg)
	}
}

func TestFromEnvOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_URL", "postgres://x@y/z")
	t.Setenv("SESSION_SECRET", strings.Repeat("b", 32))
	t.Setenv("PORT", "3012")
	t.Setenv("RULINKY_ENV", "production")
	t.Setenv("RULINKY_DISABLE_WORKER", "1")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != "postgres://x@y/z" || cfg.Port != "3012" || !cfg.Production || !cfg.DisableWorker {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestFromEnvRequiresLongSessionSecret(t *testing.T) {
	clearEnv(t)
	t.Setenv("SESSION_SECRET", "short")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected an error for a short SESSION_SECRET")
	}
}

func TestFromEnvNoSecretSkipsSecretCheck(t *testing.T) {
	clearEnv(t)
	if _, err := FromEnvNoSecret(); err != nil {
		t.Fatalf("crawl path must not need SESSION_SECRET: %v", err)
	}
}
