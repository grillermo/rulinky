package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Port                   string
	DatabaseURL            string
	SessionSecret          []byte
	Production             bool
	DisableWorker          bool
	SlackOTPWebhookURL     string
	ReaditsoonEmail        string
	ChromeRealUserDataDir  string
	ChromeUserDataDir      string
	ChromeProfileDirectory string
}

// FromEnv builds the config from environment variables (call LoadDotenv
// first). An empty variable counts as unset.
func FromEnv() (Config, error) {
	secret := os.Getenv("SESSION_SECRET")
	if len(secret) < 32 {
		return Config{}, errors.New("SESSION_SECRET must be set to at least 32 characters (generate one with: openssl rand -hex 32)")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, err
	}
	return Config{
		Port:                   envOr("PORT", "3002"),
		DatabaseURL:            databaseURL(),
		SessionSecret:          []byte(secret),
		Production:             os.Getenv("RULINKY_ENV") == "production",
		DisableWorker:          os.Getenv("RULINKY_DISABLE_WORKER") == "1",
		SlackOTPWebhookURL:     os.Getenv("SLACK_OTP_WEBHOOK_URL"),
		ReaditsoonEmail:        envOr("READITSOON_EMAIL", "guillermo.siliceo@kindle.com"),
		ChromeRealUserDataDir:  envOr("CHROME_REAL_USER_DATA_DIR", filepath.Join(home, "Library/Application Support/Google/Chrome")),
		ChromeUserDataDir:      envOr("CHROME_USER_DATA_DIR", filepath.Join(home, "Library/Application Support/Google/ChromeRulinky")),
		ChromeProfileDirectory: envOr("CHROME_PROFILE_DIRECTORY", "Default"),
	}, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// databaseURL returns DATABASE_URL, or a libpq keyword DSN built from the
// DATABASE_* variables the Rails app used (database name defaults to rulinky).
func databaseURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	parts := []string{}
	add := func(key, value string) {
		if value != "" {
			parts = append(parts, key+"="+quoteDSN(value))
		}
	}
	add("host", os.Getenv("DATABASE_HOST"))
	add("port", envOr("DATABASE_PORT", "5432"))
	add("user", os.Getenv("DATABASE_USER"))
	add("password", os.Getenv("DATABASE_PASSWORD"))
	add("dbname", envOr("DATABASE_NAME", "rulinky"))
	return strings.Join(parts, " ")
}

func quoteDSN(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `'`, `\'`)
	return "'" + v + "'"
}
