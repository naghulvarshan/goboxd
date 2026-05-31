package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/thesouldev/goboxd/internal/programs"
	"github.com/thesouldev/goboxd/internal/types"
)

var config *types.Config

var langMap map[string]types.LanguageSettings

func writeResponse(resp string, w http.ResponseWriter, status int) {
	w.WriteHeader(status)
	w.Write([]byte(resp))
}

func healthz(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	slog.Info("API Report", "path", "/healthz", "method", "GET")
	writeResponse(`{"status":"ok"}`, w, http.StatusOK)
}

func readyz(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	slog.Info("API Report", "path", "/readyz", "method", "GET")
	readyzRes := types.ReadyzResponse{
		Status: "success",
		Nsjail: types.SmokeTestRes{
			Ok:      true,
			Version: &[]string{os.Getenv("NSJAIL_VERSION")}[0],
		},
		Languages: make(map[string]types.SmokeTestRes),
	}
	for i := range config.LanguageSettings {
		args := strings.Fields(config.LanguageSettings[i].VersionCmd)
		if len(args) == 0 {
			readyzRes.Languages[config.LanguageSettings[i].Id] = types.SmokeTestRes{
				Ok:    false,
				Error: &[]string{"version command not configured"}[0],
			}
			continue
		}
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		var langRes types.SmokeTestRes
		if err != nil {
			errStr := err.Error()
			langRes = types.SmokeTestRes{
				Ok:    false,
				Error: &errStr,
			}
			readyzRes.Languages[config.LanguageSettings[i].Id] = langRes
			continue
		}
		v := string(out)
		v = strings.TrimSuffix(v, "\n")
		langRes = types.SmokeTestRes{
			Ok:      true,
			Version: &v,
		}
		readyzRes.Languages[config.LanguageSettings[i].Id] = langRes
	}
	res, _ := json.Marshal(readyzRes)
	writeResponse(string(res), w, http.StatusOK)
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
	if langSettings, ok := langMap[req.Language]; !ok {
		errorResp(InvalidInputError{types.PreBuildError{
			ErrorDetails: types.ErrorDetails{
				Code:    types.UnkownLanugageErrCode,
				Message: "langauge is unkown",
			},
		}}, w)
		return
	} else {
		out, err := programs.Run(req, config.DefaultCommonSettings["nsjail_args"], langSettings)
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
	router.GET("/readyz", readyz)
	router.POST("/run", runProgram)
	go junkCleaner(context.TODO())
	langMap = make(map[string]types.LanguageSettings)
	for i := range cfg.LanguageSettings {
		id := cfg.LanguageSettings[i].Id
		if _, ok := langMap[id]; ok {
			slog.Error("duplicate language def in config", "langugae", id)
		}
		langMap[cfg.LanguageSettings[i].Id] = cfg.LanguageSettings[i]
	}
	slog.Info("Started server", "port", port, "address", "http://localhost")
	log.Fatal(http.ListenAndServe(":"+port, router))
}
