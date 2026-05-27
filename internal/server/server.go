package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/thesouldev/goboxd/internal/programs"
	"github.com/thesouldev/goboxd/internal/types"
)

var config *types.Config

func writeResponse(resp string, w http.ResponseWriter, status int) {
	w.WriteHeader(status)
	w.Write([]byte(resp))
}

func healthz(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	slog.Info("API Report", "path", "/healthz", "method", "GET")
	writeResponse(`{"status":"ok"}`, w, http.StatusOK)
}

func runProgram(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	slog.Info("API Report", "path", "/run", "method", "POST")
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
	if config == nil || config.LanguageSettings == nil {
		errorResp(InternalServerError{errors.New("internal server error")}, w)
		return
	}
	if langSettings, ok := config.LanguageSettings[req.Language]; !ok {
		errorResp(InvalidInputError{types.PreBuildError{
			ErrorDetails: types.ErrorDetails{
				Code:    types.UnkownLanugageErrCode,
				Message: "langauge is unkown",
			},
		}}, w)
		return
	} else {
		out, err := programs.Run(req, config.DefaultCommonSettings["nsjail_args"], langSettings)

		log.Println(err)
		if err != nil {
			errorResp(err, w)
			return
		}
		resp, _ := json.Marshal(out)
		writeResponse(string(resp), w, http.StatusOK)
	}

}

func configureRouter() *httprouter.Router {
	router := httprouter.New()
	return router
}

func Serve(port string, cfg *types.Config) {
	config = cfg
	router := configureRouter()
	router.GET("/healthz", healthz)
	router.POST("/run", runProgram)
	slog.Info("Started server", "port", port, "address", "http://localhost")
	log.Fatal(http.ListenAndServe(":"+port, router))
}
