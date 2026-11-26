package saveimport

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloneTreeAndCommitTree(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	staged := filepath.Join(root, "staged", "Cluster_1")
	target := filepath.Join(root, "target", "Cluster_1")
	backup := filepath.Join(root, "backup", "Cluster_1")

	if err := os.MkdirAll(filepath.Join(source, "Master", "save"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "cluster.ini"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "Master", "save", "world"), []byte("world"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CloneTree(source, staged); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "cluster.ini"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := CommitTree(staged, target, backup); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(target, "cluster.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("staged cluster.ini = %q, want new", got)
	}
	got, err = os.ReadFile(filepath.Join(target, "Master", "save", "world"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "world" {
		t.Fatalf("cloned save = %q, want world", got)
	}
}
