package config

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotenv reads dir/.env and then dir/.env.local into the process
// environment. .env.local overrides .env; variables that were already set in
// the process environment before the call always win. Format: KEY=VALUE per
// line, blank lines and # comments ignored, optional surrounding double quotes.
func LoadDotenv(dir string) error {
	preserved := map[string]bool{}
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		preserved[k] = true
	}
	for _, name := range []string{".env", ".env.local"} {
		if err := loadFile(filepath.Join(dir, name), preserved); err != nil {
			return err
		}
	}
	return nil
}

func loadFile(path string, preserved map[string]bool) error {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
			v = v[1 : len(v)-1]
		}
		if preserved[k] {
			continue
		}
		if err := os.Setenv(k, v); err != nil {
			return err
		}
	}
	return sc.Err()
}
