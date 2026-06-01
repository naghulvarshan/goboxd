package server

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func TestIdGenerator(t *testing.T) {
	id, err := idGenerator()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(id) != 5 {
		t.Errorf("id length = %d, want 5", len(id))
	}
	for _, c := range id {
		if !unicode.IsLower(c) && !unicode.IsDigit(c) {
			t.Errorf("id contains non-alphanumeric char: %q", c)
		}
	}
}

func TestIdGeneratorUniqueness(t *testing.T) {
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		id, err := idGenerator()
		if err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
		}
		if seen[id] {
			t.Errorf("duplicate id generated: %s", id)
		}
		seen[id] = true
	}
}

func TestGenerateWorkSpace(t *testing.T) {
	home, _ := os.UserHomeDir()
	os.MkdirAll(filepath.Join(home, "nsjail_programs"), 0755)

	baseDir, id, err := generateWorkSpace()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(baseDir) })

	if id == "" {
		t.Error("expected non-empty id")
	}
	if _, err := os.Stat(baseDir); err != nil {
		t.Errorf("baseDir not created: %v", err)
	}
	if _, err := os.Stat(baseDir + "/proc"); err != nil {
		t.Errorf("proc subdirectory not created: %v", err)
	}
	if !strings.Contains(baseDir, id) {
		t.Errorf("baseDir %q does not contain id %q", baseDir, id)
	}
}

func TestGenerateWorkSpaceUnique(t *testing.T) {
	home, _ := os.UserHomeDir()
	os.MkdirAll(filepath.Join(home, "nsjail_programs"), 0755)

	dir1, id1, err := generateWorkSpace()
	if err != nil {
		t.Fatalf("first workspace: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir1) })

	dir2, id2, err := generateWorkSpace()
	if err != nil {
		t.Fatalf("second workspace: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir2) })

	if id1 == id2 {
		t.Errorf("two workspaces got the same id %q", id1)
	}
	if dir1 == dir2 {
		t.Errorf("two workspaces got the same directory %q", dir1)
	}
}

func TestAddSource(t *testing.T) {
	dir := t.TempDir()
	content := "print('hello')"

	if err := addSource("solution.py", dir, content); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "solution.py"))
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(got) != content {
		t.Errorf("file content = %q, want %q", string(got), content)
	}
}

func TestAddSourceError(t *testing.T) {
	err := addSource("test.py", "/nonexistent/dir", "content")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(InternalServerError); !ok {
		t.Errorf("expected InternalServerError, got %T", err)
	}
}

func TestAddSourceOverwrites(t *testing.T) {
	dir := t.TempDir()
	addSource("file.py", dir, "original")
	addSource("file.py", dir, "updated")

	got, _ := os.ReadFile(filepath.Join(dir, "file.py"))
	if string(got) != "updated" {
		t.Errorf("file content = %q, want %q", string(got), "updated")
	}
}

func TestCreateTestWS(t *testing.T) {
	dir := t.TempDir()
	tests := []Tests{
		{Stdin: "3\n", ExpectedOut: "odd\n"},
		{Stdin: "4\n", ExpectedOut: "even\n"},
	}

	if err := createTestWS(dir, tests); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, tc := range tests {
		testDir := filepath.Join(dir, fmt.Sprintf("test_%d", i))
		if _, err := os.Stat(testDir); err != nil {
			t.Errorf("test_%d directory not created: %v", i, err)
		}
		got, err := os.ReadFile(filepath.Join(testDir, "input"))
		if err != nil {
			t.Fatalf("test_%d input file not created: %v", i, err)
		}
		if string(got) != tc.Stdin {
			t.Errorf("test_%d input = %q, want %q", i, string(got), tc.Stdin)
		}
	}
}

func TestCreateTestWSEmptyStdin(t *testing.T) {
	dir := t.TempDir()
	if err := createTestWS(dir, []Tests{{Stdin: "", ExpectedOut: "hi\n"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "test_0/input"))
	if string(got) != "" {
		t.Errorf("expected empty input file, got %q", string(got))
	}
}

func TestCreateTestWSError(t *testing.T) {
	err := createTestWS("/nonexistent/dir", []Tests{{Stdin: "a", ExpectedOut: "b"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if _, ok := err.(InternalServerError); !ok {
		t.Errorf("expected InternalServerError, got %T", err)
	}
}
