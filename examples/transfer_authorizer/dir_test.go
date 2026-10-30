package transferauthorizer_test

import (
	"os"
	"path/filepath"
	"testing"

	transferauthorizer "github.com/getmanza/manza-go/examples/transfer_authorizer"
)

func TestDirIsTheExampleDirectoryWhateverTheWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())

	if _, err := os.Stat(filepath.Join(transferauthorizer.Dir(), ".env.example")); err != nil {
		t.Fatalf("expected .env.example in %s: %v", transferauthorizer.Dir(), err)
	}
}
