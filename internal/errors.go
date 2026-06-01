package server

import (
	"net/http"
)

type InvalidInputError struct {
	Message string
}

type InternalServerError struct {
	Message string
}

func (i InvalidInputError) Error() string {
	return "invalid input: " + i.Message
}

func (i InternalServerError) Error() string {
	return "internal server error"
}

func errorResp(err error, w http.ResponseWriter) {
	statusCode := http.StatusInternalServerError
	switch err.(type) {
	case InvalidInputError:
		statusCode = http.StatusBadRequest
	}
	w.WriteHeader(statusCode)
	w.Write([]byte(err.Error()))
}
