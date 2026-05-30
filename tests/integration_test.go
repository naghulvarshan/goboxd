package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ghodss/yaml"
)

type testFile struct {
	Request  apiRequest       `json:"request"`
	Expected expectedResponse `json:"expected"`
}

type apiRequest struct {
	Language         string          `json:"language"`
	Source           string          `json:"source"`
	SourceFileName   string          `json:"source_filename,omitempty"`
	ArtifactFileName string          `json:"artifact_filename,omitempty"`
	Build            *limitsAndFlags `json:"build,omitempty"`
	Run              limitsAndFlags  `json:"run"`
	Tests            []testCase      `json:"tests"`
}

type limitsAndFlags struct {
	Limits limits   `json:"limits"`
	Flags  []string `json:"flags,omitempty"`
}

type limits struct {
	WallTime     int   `json:"wall_time_s"`
	MemoryKB     int64 `json:"memory_kb"`
	MaxProcesses int   `json:"max_processes"`
}

type testCase struct {
	Stdin       string `json:"stdin"`
	ExpectedOut string `json:"expected_stdout"`
}

type expectedResponse struct {
	Status string         `json:"status"`
	Tests  []expectedTest `json:"tests,omitempty"`
}

type expectedTest struct {
	Status string `json:"status"`
	Stdout string `json:"stdout,omitempty"`
}

type runResponse struct {
	Status string       `json:"status"`
	Tests  []testOutput `json:"test"`
}

type testOutput struct {
	Status       string `json:"status"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	DurationMs   int    `json:"duration_ms"`
	MemoryPeakKb int64  `json:"memory_peak_kb"`
}

// ── test driver ───────────────────────────────────────────────────────────────

// TestIntegration discovers every testcases/<language>/<name>.yaml file and
// runs it as TestIntegration/<Language>/<Name>.
//
// Filter examples:
//
//	go test -tags=integration -run "TestIntegration/Java"           # all Java
//	go test -tags=integration -run "TestIntegration/Java/HelloWorld"
func TestIntegration(t *testing.T) {
	type entry struct{ name, path string }
	byLang := map[string][]entry{}

	err := filepath.WalkDir("testcases", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(path) != ".yaml" {
			return err
		}
		parts := strings.Split(filepath.ToSlash(path), "/")
		if len(parts) != 3 { // testcases/<lang>/<name>.yaml
			return nil
		}
		lang := parts[1]
		name := strings.TrimSuffix(parts[2], ".yaml")
		byLang[lang] = append(byLang[lang], entry{name, path})
		return nil
	})
	if err != nil {
		t.Fatalf("walk testcases/: %v", err)
	}
	if len(byLang) == 0 {
		t.Fatal("no test cases found under testcases/")
	}

	langs := make([]string, 0, len(byLang))
	for l := range byLang {
		langs = append(langs, l)
	}
	sort.Strings(langs)

	for _, lang := range langs {
		entries := byLang[lang]
		sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
		t.Run(toPascal(lang), func(t *testing.T) {
			for _, e := range entries {
				e := e
				t.Run(toPascal(e.name), func(t *testing.T) {
					runCase(t, e.path)
				})
			}
		})
	}
}

func runCase(t *testing.T, path string) {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var tf testFile
	if err := yaml.Unmarshal(raw, &tf); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	resp := postRun(t, tf.Request)

	if resp.Status != tf.Expected.Status {
		t.Errorf("response.status = %q, want %q", resp.Status, tf.Expected.Status)
	}
	if len(tf.Expected.Tests) == 0 {
		return
	}
	if len(resp.Tests) != len(tf.Expected.Tests) {
		t.Fatalf("got %d test results, want %d", len(resp.Tests), len(tf.Expected.Tests))
	}
	for i, exp := range tf.Expected.Tests {
		got := resp.Tests[i]
		if got.Status != exp.Status {
			t.Errorf("test[%d].status = %q, want %q  stderr: %s", i, got.Status, exp.Status, got.Stderr)
		}
		// Only assert stdout on success — errored/wrong_output stdout is noisy
		if exp.Status == "success" && exp.Stdout != "" && got.Stdout != exp.Stdout {
			t.Errorf("test[%d].stdout mismatch\ngot:  %q\nwant: %q", i, got.Stdout, exp.Stdout)
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func baseURL() string {
	if u := os.Getenv("GOBOXD_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func postRun(t *testing.T, req apiRequest) runResponse {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(baseURL()+"/run", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /run: %v  (is the server running at %s?)", err, baseURL())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var raw map[string]any
		json.NewDecoder(resp.Body).Decode(&raw)
		t.Fatalf("expected 200, got %d: %v", resp.StatusCode, raw)
	}
	var result runResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return result
}

// toPascal converts "hello-world" or "hello_world" → "HelloWorld".
func toPascal(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
	var b strings.Builder
	for _, p := range parts {
		if len(p) > 0 {
			b.WriteString(strings.ToUpper(p[:1]))
			b.WriteString(p[1:])
		}
	}
	return b.String()
}
