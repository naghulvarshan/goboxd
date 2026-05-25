package types

import (
	"encoding/json"
	"fmt"
)

const (
	BadJsonErrCode           = "bad_json"
	InvalidFileNameErrCode   = "invalid_filename"
	InvalidSourceErrCode     = "invalid_source"
	InvalidFieldErrCode      = "invalid_field"
	UnkownLanugageErrCode    = "unkown_lanugage"
	MalformedFileNameErrCode = "malformed_filename"
	OverSizedBodyErrCode     = "oversized_body"
	DisallowedFlageErrCode   = "disallowed_flag"
)

type ProgramInfo struct {
	Language         string          `json:"language"`
	Source           string          `json:"source"`
	SourceFileName   *string         `json:"source_filename,omitempty"`
	ArtifaceFileName *string         `json:"artifact_filename,omitempty"`
	Build            *LimitsAndFlags `json:"build,omitempty"`
	Run              *LimitsAndFlags `json:"run,omitempty"`
	Tests            []Tests         `json:"tests"`
}

type LimitsAndFlags struct {
	Limits Limits   `json:"limits"`
	Flags  []string `json:"flags"`
}

type Limits struct {
	WallTime     int   `json:"wall_time_s"`
	MemoryKB     int64 `json:"memory_kb"`
	MaxProcesses int   `json:"max_processes"`
}

type Tests struct {
	Stdin       string `json:"stdin"`
	ExpectedOut string `json:"expected_stdout"`
}

type Response struct {
	Status     string           `json:"status"`
	Build      ExecutionDetails `json:"build"`
	TestOutput TestOutput       `json:"test"`
}

type ExecutionDetails struct {
	Status   string `json:"status"`
	STDOut   string `json:"stdout"`
	STDErr   string `json:"stderr"`
	Duration int    `json:"duration_ms"`
}

type TestOutput struct {
	ExecutionDetails
	MemoryPeakKb int64 `json:"memory_peak_kb"`
}

type PreBuildError struct {
	ErrorDetails ErrorDetails `json:"error"`
}

type ErrorDetails struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (pbe PreBuildError) Error() string {
	out, _ := json.Marshal(pbe) //Ignoring error since we are marshalling a known struct
	return string(out)
}

func UnmarshallRequest(body []byte) (*ProgramInfo, error) {
	var req *ProgramInfo
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, PreBuildError{
			ErrorDetails: ErrorDetails{
				Code:    BadJsonErrCode,
				Message: fmt.Sprintf("error parsing request body: %v", err),
			},
		}
	}
	if req.Language == "" {
		return nil, PreBuildError{
			ErrorDetails: ErrorDetails{
				Code:    UnkownLanugageErrCode,
				Message: "langauge is unkown",
			},
		}
	}
	if req.Source == "" {
		return nil, PreBuildError{
			ErrorDetails: ErrorDetails{
				Code:    InvalidSourceErrCode,
				Message: "invalid source",
			},
		}
	}
	if len(req.Tests) == 0 {
		return nil, PreBuildError{
			ErrorDetails: ErrorDetails{
				Code:    InvalidFieldErrCode,
				Message: "at least one test case is required",
			},
		}
	}
	return req, nil
}
