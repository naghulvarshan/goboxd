package server

import (
	"io"
	"log"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/thesouldev/goboxd/internal/programs"
	"github.com/thesouldev/goboxd/internal/types"
)

func writeResponse(resp string, w http.ResponseWriter, status int) {
	w.WriteHeader(status)
	w.Write([]byte(resp))
}

func healthz(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	writeResponse(`{"status":"ok"}`, w, http.StatusOK)
}

func runProgram(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		errorResp(InvalidInputError{err}, w)
		return
	}
	req, err := types.UnmarshallRequest(reqBody)
	if err != nil {
		errorResp(InvalidInputError{err}, w)
		return
	}
	err = programs.Run(req)
	if err != nil {
		errorResp(err, w)
	}
}

func configureRouter() *httprouter.Router {
	router := httprouter.New()
	return router
}

func Serve(port string) {
	router := configureRouter()
	router.GET("/healthz", healthz)
	router.POST("/run", runProgram)
	log.Println("Started serving on port:", port)
	log.Fatal(http.ListenAndServe(":"+port, router))
}
