package pokeapi_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// corruptAll overwrites every file under dir with invalid JSON.
func corruptAll(t *testing.T, dir string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		return os.WriteFile(p, []byte("{not json"), 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}
