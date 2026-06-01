package server

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
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
	TakeFromRequest          = "TAKE_FROM_REQUEST"
	SourceMaxLimit           = 256 * 1025 // Restricting maximum size of source file
	MaxTestCases             = 50
	MaxJobs                  = 10 // TODO: implement this
)

type ProgramInfo struct {
	Language             string          `json:"language"`
	Source               string          `json:"source"`
	SourceFileName       *string         `json:"source_filename,omitempty"`
	ArtifaceFileName     *string         `json:"artifact_filename,omitempty"`
	Build                *LimitsAndFlags `json:"build,omitempty"`
	Run                  *LimitsAndFlags `json:"run,omitempty"`
	Tests                []Tests         `json:"tests"`
	EvaluationScript     *string         `json:"evaluation_script"`
	EvaluationScriptLang *string         `json:"evaluation_script_lang"`
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
	Status               string            `json:"status"`
	Build                *ExecutionDetails `json:"build,omitempty"`
	TestOutputs          []TestOutput      `json:"test,omitempty"`
	EvaluationResultJSON *string           `json:"evaluation_result_json,omitempty"`
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
	if len(req.Tests) == 0 && req.EvaluationScript == nil {
		return nil, PreBuildError{
			ErrorDetails: ErrorDetails{
				Code:    InvalidFieldErrCode,
				Message: "at least one test case is required",
			},
		}
	}
	if len(req.Tests) > MaxTestCases {
		return nil, PreBuildError{
			ErrorDetails: ErrorDetails{
				Code: InvalidFieldErrCode,
				Message: "the maximum number of test case can only be " +
					strconv.FormatInt(int64(MaxTestCases), 10),
			},
		}
	}
	if len(req.Source) > SourceMaxLimit {
		return nil, PreBuildError{
			ErrorDetails: ErrorDetails{
				Code:    "max_limit_exceeded",
				Message: "maximum size for source is 256KiB",
			},
		}
	}
	if req.SourceFileName != nil {
		if *req.SourceFileName == "" {
			return nil, PreBuildError{
				ErrorDetails: ErrorDetails{
					Code:    InvalidFileNameErrCode,
					Message: "filename cannot be empty string",
				},
			}
		}
		if len(*req.SourceFileName) > 255 {
			return nil, PreBuildError{
				ErrorDetails: ErrorDetails{
					Code:    InvalidFileNameErrCode,
					Message: "filename is too long",
				},
			}
		}
		// block path traversal
		if strings.Contains(*req.SourceFileName, "/") || strings.Contains(*req.SourceFileName, "..") {
			return nil, PreBuildError{
				ErrorDetails: ErrorDetails{
					Code:    InvalidFileNameErrCode,
					Message: "filename contains / or .. ",
				},
			}
		}
		// only allow alphanumeric, dash, underscore, dot
		for _, c := range *req.SourceFileName {
			if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '_' && c != '.' {
				return nil, PreBuildError{
					ErrorDetails: ErrorDetails{
						Code:    InvalidFileNameErrCode,
						Message: "filename contains invalid characters. only alphanumeric, dash, underscore, dot characters are allowed",
					},
				}
			}
		}
	}
	if req.EvaluationScript != nil && (req.EvaluationScriptLang == nil || *req.EvaluationScriptLang == "") {
		return nil, PreBuildError{
			ErrorDetails: ErrorDetails{
				Code:    InvalidFileNameErrCode,
				Message: "if evaluation_script is specified, evaluation_script_lang must be set to a valid language",
			},
		}
	}
	return req, nil
}

type Config struct {
	Version               string             `json:"version"`
	NsjailPath            string             `json:"nsjail_path"`
	DefaultCommonSettings map[string]string  `json:"default_common_settings"`
	LanguageSettings      []LanguageSettings `json:"languages"`
}

type LanguageSettings struct {
	Id             string   `json:"id"`
	Name           string   `json:"name"`
	Source         string   `json:"source"`
	BinaryFileName *string  `json:"artifact,omitempty"`
	BuildOpts      *Options `json:"build,omitempty"`
	RunOpts        Options  `json:"run"`
	VersionCmd     *string  `json:"version_cmd,omitempty"`
}

type Options struct {
	Cmd            string   `json:"cmd"`
	Args           []string `json:"args,omitempty"`
	ResourceLimits Limits   `json:"limits"`
}

type ResourceLimit struct {
	WalltTime_S int `json:"wall_time_s"`
	MaxProc     int `json:"max_processes"`
	MemLimit    int `json:"memory_kb"`
}

type ReadyzResponse struct {
	Status    string                  `json:"status"`
	Nsjail    SmokeTestRes            `json:"nsjail"`
	Languages map[string]SmokeTestRes `json:"languages"`
}

type SmokeTestRes struct {
	Ok      bool    `json:"ok"`
	Version *string `json:"version,omitempty"`
	Error   *string `json:"error,omitempty"`
}

// TODO: Make all fields proper struct with def
type InfoResp struct {
	BuildInfo map[string]string      `json:"build_info"`
	Nsjail    map[string]string      `json:"nsjail"`
	Languages []json.RawMessage      `json:"lanugages"`
	Limits    map[string]interface{} `json:"limits"`
	Stats     map[string]interface{} `json:"stats"`
}
