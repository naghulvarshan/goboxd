package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/julienschmidt/httprouter"
)

var config *Config

var langMap map[string]LanguageSettings

var activeRequests int64 // To keep track of active requests
var jobsTotal, jobsFailedWithIntSvrErr atomic.Int64
var lastInternalErr *time.Time
var mu sync.RWMutex

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func newRequestID() string {
	return fmt.Sprintf("%08x", rand.Uint32())
}

func logMiddleware(next httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		reqID := newRequestID()
		next(rec, r, p)
		slog.Info("api request",
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}
}

func writeResponse(resp string, w http.ResponseWriter, status int) {
	w.WriteHeader(status)
	w.Write([]byte(resp))
}

func healthz(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	writeResponse(`{"status":"ok"}`, w, http.StatusOK)
}

func readyz(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	readyzRes := ReadyzResponse{
		Status: "success",
		Nsjail: SmokeTestRes{
			Ok:      true,
			Version: &[]string{os.Getenv("NSJAIL_VERSION")}[0],
		},
		Languages: make(map[string]SmokeTestRes),
	}
	var errored bool
	// Run version cmd for each language supported
	for i := range config.LanguageSettings {
		if config.LanguageSettings[i].VersionCmd == nil {
			readyzRes.Languages[config.LanguageSettings[i].Id] = SmokeTestRes{
				Ok:    false,
				Error: &[]string{"version command not configured"}[0],
			}
			continue
		}
		args := strings.Fields(*config.LanguageSettings[i].VersionCmd)
		if len(args) == 0 {
			readyzRes.Languages[config.LanguageSettings[i].Id] = SmokeTestRes{
				Ok:    false,
				Error: &[]string{"version command not configured"}[0],
			}
			continue
		}
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		var langRes SmokeTestRes
		if err != nil {
			errored = true
			errStr := err.Error()
			langRes = SmokeTestRes{
				Ok:    false,
				Error: &errStr,
			}
			readyzRes.Languages[config.LanguageSettings[i].Id] = langRes
			continue
		}
		v := string(out)
		v = strings.TrimSuffix(v, "\n")
		langRes = SmokeTestRes{
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
	resp := InfoResp{
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
			"max_source_bytes":    SourceMaxLimit,
			"max_tests":           MaxTestCases,
			"max_concurrent_jobs": MaxJobs, // TODO: need to implement
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
		resp.Stats["last_internal_error_at"] = lastInternalErr.Format(time.DateTime)
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
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		errorResp(InvalidInputError{Message: err.Error()}, w)
		return
	}
	req, err := UnmarshallRequest(reqBody)
	if err != nil {
		errorResp(InvalidInputError{Message: err.Error()}, w)
		return
	}
	if config == nil || config.LanguageSettings == nil {
		errorResp(InternalServerError{}, w)
		mu.Lock()
		defer mu.Unlock()
		now := time.Now()
		lastInternalErr = &now
		jobsFailedWithIntSvrErr.Add(1)
		return
	}
	if langSettings, ok := langMap[req.Language]; !ok {
		errorResp(InvalidInputError{Message: errToString(PreBuildError{
			ErrorDetails: ErrorDetails{
				Code:    UnkownLanugageErrCode,
				Message: "langauge is unkown",
			},
		})}, w)
		return
	} else {
		atomic.AddInt64(&activeRequests, 1)
		jobsTotal.Add(1)
		defer atomic.AddInt64(&activeRequests, -1)
		out, err := Run(req, config.DefaultCommonSettings["nsjail_args"], langSettings)
		if err != nil {
			errorResp(err, w)
			switch err.(type) {
			case InternalServerError:
				mu.Lock()
				defer mu.Unlock()
				now := time.Now()
				lastInternalErr = &now
				jobsFailedWithIntSvrErr.Add(1)
			}
			return
		}
		resp, _ := json.Marshal(out)
		writeResponse(string(resp), w, http.StatusOK)
	}

}

func errToString(pe PreBuildError) string {
	out, _ := json.Marshal(pe)
	return string(out)
}

func configureRouter() *httprouter.Router {
	router := httprouter.New()
	return router
}

func Serve(port string, cfg *Config) {
	config = cfg
	router := configureRouter()
	router.GET("/healthz", logMiddleware(healthz))
	router.GET("/readyz", logMiddleware(readyz))
	router.POST("/run", logMiddleware(runProgram))
	router.GET("/info", logMiddleware(info))
	go junkCleaner(context.TODO())
	langMap = make(map[string]LanguageSettings)
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
