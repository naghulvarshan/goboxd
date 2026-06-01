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
	"sync"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/thesouldev/goboxd/internal/programs"
	"github.com/thesouldev/goboxd/internal/types"
)

var config *types.Config

var langMap map[string]types.LanguageSettings

var activeRequests int64 // To keep track of active requests
var jobsTotal, jobsFailedWithIntSvrErr atomic.Int64
var lastInternalErr *time.Time
var mu sync.RWMutex

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
	var errored bool
	// Run version cmd for each language supported
	for i := range config.LanguageSettings {
		if config.LanguageSettings[i].VersionCmd == nil {
			readyzRes.Languages[config.LanguageSettings[i].Id] = types.SmokeTestRes{
				Ok:    false,
				Error: &[]string{"version command not configured"}[0],
			}
			continue
		}
		args := strings.Fields(*config.LanguageSettings[i].VersionCmd)
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
			errored = true
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
	returnStatus := http.StatusOK
	if errored {
		returnStatus = http.StatusServiceUnavailable
		readyzRes.Status = "degraded"
	}
	res, _ := json.Marshal(readyzRes)
	writeResponse(string(res), w, returnStatus)
}

func info(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	slog.Info("API Report", "path", "/info", "method", "GET")
	resp := types.InfoResp{
		BuildInfo: map[string]string{
			"version":    config.Version,
			"commit":     os.Getenv("GIT_COMMIT"),
			"go_version": os.Getenv("GO_VERSION"),
		},
		Nsjail: map[string]string{
			"path":    config.NsjailPath,
			"version": os.Getenv("NSJAIL_VERSION"),
		},
		Languages: make([]json.RawMessage, 0),
		Limits: map[string]interface{}{
			"max_source_bytes":    types.SourceMaxLimit,
			"max_tests":           types.MaxTestCases,
			"max_concurrent_jobs": types.MaxJobs, // TODO: need to implement
		},
		Stats: map[string]interface{}{
			"in_flight_jobs":       activeRequests,
			"jobs_total":           jobsTotal.Load(),
			"jobs_failed_internal": jobsFailedWithIntSvrErr.Load(),
		},
	}
	mu.RLock()
	defer mu.RUnlock()
	if lastInternalErr != nil {
		resp.Stats["last_internal_server_error"] = lastInternalErr.Format(time.DateTime)
	}
	for _, lang := range config.LanguageSettings {
		lang.VersionCmd = nil
		info, _ := json.Marshal(lang)
		resp.Languages = append(resp.Languages, info)
	}
	out, _ := json.Marshal(resp)
	writeResponse(string(out), w, http.StatusOK)
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
		atomic.AddInt64(&activeRequests, 1)
		jobsTotal.Add(1)
		defer atomic.AddInt64(&activeRequests, -1)
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
	router.GET("/info", info)
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
