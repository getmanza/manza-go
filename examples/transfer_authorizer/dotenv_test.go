package transferauthorizer_test

import (
	"os"
	"path/filepath"
	"testing"

	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

func TestLoadDotenvOverridesTheEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\n\nMANZA_EXAMPLE_A=from-file\nexport MANZA_EXAMPLE_B=\"quoted value\"\n" +
		"MANZA_EXAMPLE_C=plain # trailing comment\nMANZA_EXAMPLE_D=\nnot a pair\n" +
		"MANZA_EXAMPLE_E=\"whsec_x\" # the signing secret\nMANZA_EXAMPLE_F='a # b'\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MANZA_EXAMPLE_A", "stale-export")
	t.Setenv("MANZA_EXAMPLE_B", "")
	t.Setenv("MANZA_EXAMPLE_C", "")
	t.Setenv("MANZA_EXAMPLE_D", "set")
	t.Setenv("MANZA_EXAMPLE_E", "")
	t.Setenv("MANZA_EXAMPLE_F", "")

	if err := transferauthorizer.LoadDotenv(path); err != nil {
		t.Fatalf("load: %v", err)
	}
	for name, want := range map[string]string{
		"MANZA_EXAMPLE_A": "from-file", "MANZA_EXAMPLE_B": "quoted value", "MANZA_EXAMPLE_C": "plain", "MANZA_EXAMPLE_D": "",
		"MANZA_EXAMPLE_E": "whsec_x", "MANZA_EXAMPLE_F": "a # b",
	} {
		if got := os.Getenv(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestLoadDotenvIgnoresAMissingFile(t *testing.T) {
	if err := transferauthorizer.LoadDotenv(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Fatalf("load: %v", err)
	}
}
