package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteResponse(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
	}{
		{"ok", `{"status":"ok"}`, http.StatusOK},
		{"created", `{"id":"123"}`, http.StatusCreated},
		{"bad request", `{"error":"bad"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeResponse(tc.body, rr, tc.status)
			if rr.Code != tc.status {
				t.Errorf("status = %d, want %d", rr.Code, tc.status)
			}
			if rr.Body.String() != tc.body {
				t.Errorf("body = %q, want %q", rr.Body.String(), tc.body)
			}
		})
	}
}

func TestHealthz(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthz(rr, req, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if rr.Body.String() != `{"status":"ok"}` {
		t.Errorf("body = %q, want %q", rr.Body.String(), `{"status":"ok"}`)
	}
}

func TestErrToString(t *testing.T) {
	pbe := PreBuildError{ErrorDetails: ErrorDetails{Code: "bad_json", Message: "parse error"}}
	got := errToString(pbe)
	want := `{"error":{"code":"bad_json","message":"parse error"}}`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
