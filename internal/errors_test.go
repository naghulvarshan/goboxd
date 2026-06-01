package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInvalidInputError(t *testing.T) {
	err := InvalidInputError{Message: "bad field"}
	want := "invalid input: bad field"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestInvalidInputErrorEmpty(t *testing.T) {
	err := InvalidInputError{}
	want := "invalid input: "
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestInternalServerError(t *testing.T) {
	err := InternalServerError{Message: "db down"}
	want := "internal server error"
	if err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestErrorRespInvalidInput(t *testing.T) {
	rr := httptest.NewRecorder()
	errorResp(InvalidInputError{Message: "missing field"}, rr)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}

func TestErrorRespInternalServer(t *testing.T) {
	rr := httptest.NewRecorder()
	errorResp(InternalServerError{}, rr)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}
}
