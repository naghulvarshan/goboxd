package types

import (
	"encoding/json"
	"fmt"
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
	Status      string            `json:"status"`
	Build       *ExecutionDetails `json:"build,omitempty"`
	TestOutputs []TestOutput      `json:"test"`
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
	return req, nil
}

type Config struct {
	NsjailPath            string                      `json:"nsjail_path"`
	DefaultCommonSettings map[string]string           `json:"default_common_settings"`
	LanguageSettings      map[string]LanguageSettings `json:"language_settings"`
}

type LanguageSettings struct {
	FileName        string   `json:"filename"`
	BinaryFileName  *string  `json:"binary_filename"`
	CompilationOpts *Options `json:"compilation_options"`
	RuntimeOpts     Options  `json:"runtime_options"`
}

type Options struct {
	Path           string `json:"path"`
	Args           string `json:"args"`
	ResourceLimits Limits `json:"resource_limits"`
}

type ResourceLimit struct {
	TimeLimit    int `json:"time_limit"`
	ProcessLimit int `json:"process_limit"`
	MemLimit     int `json:"memory_limit"` // memory limit in kb
}
