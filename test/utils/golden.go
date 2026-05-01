package utils

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"sigs.k8s.io/yaml"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite golden files from current output")

// AssertGolden serializes obj as YAML and compares it to the file at path. On
// mismatch the test fails with a unified diff. Pass -update-golden to rewrite
// the file from current output.
func AssertGolden(t *testing.T, path string, obj any) {
	t.Helper()

	got, err := yaml.Marshal(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden dir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (re-run with -update-golden to create)", path, err)
	}

	if diff := cmp.Diff(string(want), string(got)); diff != "" {
		t.Errorf("golden %s mismatch (-want +got):\n%s\n\nRe-run with -update-golden to update.", path, diff)
	}
}
