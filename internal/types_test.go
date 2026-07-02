package server

import (
	"encoding/json"
	"strings"
	"testing"
)

func validBody(overrides map[string]any) []byte {
	base := map[string]any{
		"language": "py3",
		"source":   "print('hello')",
		"tests":    []map[string]any{{"stdin": "", "expected_stdout": "hello\n"}},
	}
	for k, v := range overrides {
		if v == nil {
			delete(base, k)
		} else {
			base[k] = v
		}
	}
	b, _ := json.Marshal(base)
	return b
}

func TestUnmarshallRequest(t *testing.T) {
	tests := []struct {
		name        string
		body        []byte
		wantErr     bool
		wantErrCode string
	}{
		{
			name:    "valid minimal request",
			body:    validBody(nil),
			wantErr: false,
		},
		{
			name: "valid with source_filename",
			body: validBody(map[string]any{"source_filename": "solution.py"}),
		},
		{
			name: "valid eval script replaces tests requirement",
			body: func() []byte {
				b, _ := json.Marshal(map[string]any{
					"language":               "py3",
					"source":                 "print('hi')",
					"tests":                  []any{},
					"evaluation_script":      "import sys",
					"evaluation_script_lang": "/usr/bin/python3",
				})
				return b
			}(),
		},
		{
			name:        "bad json",
			body:        []byte(`{bad`),
			wantErr:     true,
			wantErrCode: BadJsonErrCode,
		},
		{
			name:        "missing language",
			body:        validBody(map[string]any{"language": ""}),
			wantErr:     true,
			wantErrCode: UnkownLanugageErrCode,
		},
		{
			name:        "missing source",
			body:        validBody(map[string]any{"source": ""}),
			wantErr:     true,
			wantErrCode: InvalidSourceErrCode,
		},
		{
			name:        "no tests without eval script",
			body:        validBody(map[string]any{"tests": []any{}}),
			wantErr:     true,
			wantErrCode: InvalidFieldErrCode,
		},
		{
			name: "too many test cases",
			body: func() []byte {
				tc := make([]map[string]any, MaxTestCases+1)
				for i := range tc {
					tc[i] = map[string]any{"stdin": "", "expected_stdout": ""}
				}
				return validBody(map[string]any{"tests": tc})
			}(),
			wantErr:     true,
			wantErrCode: InvalidFieldErrCode,
		},
		{
			name:        "source exceeds size limit",
			body:        validBody(map[string]any{"source": strings.Repeat("a", SourceMaxLimit+1)}),
			wantErr:     true,
			wantErrCode: "max_limit_exceeded",
		},
		{
			name:        "source_filename empty string",
			body:        validBody(map[string]any{"source_filename": ""}),
			wantErr:     true,
			wantErrCode: InvalidFileNameErrCode,
		},
		{
			name:        "source_filename too long",
			body:        validBody(map[string]any{"source_filename": strings.Repeat("a", 256)}),
			wantErr:     true,
			wantErrCode: InvalidFileNameErrCode,
		},
		{
			name:        "source_filename with path separator",
			body:        validBody(map[string]any{"source_filename": "dir/file.py"}),
			wantErr:     true,
			wantErrCode: InvalidFileNameErrCode,
		},
		{
			name:        "source_filename with dot-dot traversal",
			body:        validBody(map[string]any{"source_filename": "../etc/passwd"}),
			wantErr:     true,
			wantErrCode: InvalidFileNameErrCode,
		},
		{
			name:        "source_filename with invalid character @",
			body:        validBody(map[string]any{"source_filename": "file@name.py"}),
			wantErr:     true,
			wantErrCode: InvalidFileNameErrCode,
		},
		{
			name:        "source_filename with space",
			body:        validBody(map[string]any{"source_filename": "my file.py"}),
			wantErr:     true,
			wantErrCode: InvalidFileNameErrCode,
		},
		{
			name: "eval script without lang",
			body: func() []byte {
				b, _ := json.Marshal(map[string]any{
					"language":          "py3",
					"source":            "print('hi')",
					"tests":             []any{},
					"evaluation_script": "import sys",
				})
				return b
			}(),
			wantErr:     true,
			wantErrCode: InvalidFileNameErrCode,
		},
		{
			name: "eval script with empty lang",
			body: func() []byte {
				b, _ := json.Marshal(map[string]any{
					"language":               "py3",
					"source":                 "print('hi')",
					"tests":                  []any{},
					"evaluation_script":      "import sys",
					"evaluation_script_lang": "",
				})
				return b
			}(),
			wantErr:     true,
			wantErrCode: InvalidFileNameErrCode,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnmarshallRequest(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				pbe, ok := err.(PreBuildError)
				if !ok {
					t.Fatalf("expected PreBuildError, got %T", err)
				}
				if pbe.ErrorDetails.Code != tt.wantErrCode {
					t.Errorf("error code = %q, want %q", pbe.ErrorDetails.Code, tt.wantErrCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
		})
	}
}

func TestPreBuildErrorJSON(t *testing.T) {
	pbe := PreBuildError{ErrorDetails: ErrorDetails{Code: "foo", Message: "bar"}}
	got := pbe.Error()
	want := `{"error":{"code":"foo","message":"bar"}}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnmarshallRequestReturnsCorrectFields(t *testing.T) {
	evalScript := "import sys"
	evalLang := "/usr/bin/python3"
	body, _ := json.Marshal(map[string]any{
		"language":               "py3",
		"source":                 "print('hi')",
		"tests":                  []any{},
		"evaluation_script":      evalScript,
		"evaluation_script_lang": evalLang,
	})
	got, err := UnmarshallRequest(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Language != "py3" {
		t.Errorf("Language = %q, want %q", got.Language, "py3")
	}
	if got.EvaluationScript == nil || *got.EvaluationScript != evalScript {
		t.Errorf("EvaluationScript = %v, want %q", got.EvaluationScript, evalScript)
	}
	if got.EvaluationScriptLang == nil || *got.EvaluationScriptLang != evalLang {
		t.Errorf("EvaluationScriptLang = %v, want %q", got.EvaluationScriptLang, evalLang)
	}
}
