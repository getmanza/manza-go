package transferauthorizer

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// LoadDotenv sets every KEY=VALUE line of the file at path as an environment
// variable, overriding what the shell exported (a stale export would
// otherwise mask the value in .env). A missing file is not an error. Blank
// lines and # comments are skipped; a value may be wrapped in quotes, and a
// " # comment" may follow it.
func LoadDotenv(path string) error {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') {
			// A quoted value ends at its closing quote; a comment may follow.
			if end := strings.IndexByte(value[1:], value[0]); end >= 0 {
				value = value[1 : end+1]
			}
		} else if i := strings.Index(value, " #"); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
		if err := os.Setenv(strings.TrimSpace(key), value); err != nil {
			return err
		}
	}
	return scanner.Err()
}
