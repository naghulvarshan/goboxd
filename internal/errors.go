package server

import "net/http"

type InvalidInputError struct {
	Err error
}

type InternalServerError struct {
	Err error
}

func (i InvalidInputError) Error() string {
	return i.Err.Error()
}

func (i InternalServerError) Error() string {
	return i.Err.Error()
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
