package server

import (
	"bytes"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func Run(input *ProgramInfo, defaultArgs string, languageConfig LanguageSettings) (*Response, error) {

	// Step 1: Initialize a directory to execute the code
	baseDir, id, err := generateWorkSpace()
	if err != nil {
		return nil, err
	}

	defer os.RemoveAll(baseDir) // Cleanup workspace after testing

	// Step 2: Write the program to a file
	filename := languageConfig.Source
	if input.SourceFileName != nil {
		filename = *input.SourceFileName
	}
	if err = addSource(filename, baseDir, input.Source); err != nil {
		return nil, err
	}

	output := &Response{
		Status:      "success",
		TestOutputs: []TestOutput{},
	}

	// Step 3: If evaluation script present, write it and run it directly
	if input.EvaluationScript != nil {
		if err = addSource("evaluator.script", baseDir, *input.EvaluationScript); err != nil {
			return nil, err
		}
		result := runEvaluationScript(baseDir, defaultArgs, filename, *input.EvaluationScriptLang, input.Run, languageConfig.RunOpts)
		output.EvaluationResultJSON = result
		output.Build = &ExecutionDetails{
			Status: "NOT_RUN",
		}
		return output, nil
	}

	// Step 4: Create test directories
	if err = createTestWS(baseDir, input.Tests); err != nil {
		return nil, err
	}

	// Step 5: Compile if needed
	if languageConfig.BuildOpts != nil {
		binary := languageConfig.BinaryFileName
		if binary == nil {
			binary = input.ArtifaceFileName
		}
		if binary == nil {
			return nil, errors.New("binary artifact name is required")
		}
		err := compileCode(baseDir, *binary, filename, input.Build,
			languageConfig.BuildOpts, output, defaultArgs)
		if err != nil {
			return output, nil
		}
		filename = *binary
	}

	// Step 5: Running code
	binaryFilename := filename
	if languageConfig.BuildOpts != nil {
		if languageConfig.BinaryFileName != nil && *languageConfig.BinaryFileName != "TAKE_FROM_REQUEST" {
			binaryFilename = *languageConfig.BinaryFileName
		} else if input.ArtifaceFileName != nil {
			binaryFilename = *input.ArtifaceFileName
		}
	}
	output.TestOutputs = runCode(baseDir, id, defaultArgs, binaryFilename, input.Run, languageConfig.RunOpts,
		input.Tests)
	return output, nil
}

func idGenerator() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, 5)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}

func generateWorkSpace() (string, string, error) {
	id, err := idGenerator()
	if err != nil {
		slog.Debug("error generating id", "error", err)
		return "", "", InternalServerError{}
	}
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, "nsjail_programs", "nsip_"+id)
	err = os.Mkdir(baseDir, 0755)
	if err != nil {
		if strings.Contains(err.Error(), "File exists") { // Retry once if a duplicate id is created
			slog.Debug("directory already exists, retrying...", "directory", baseDir)
			id, err = idGenerator()
			if err != nil {
				slog.Debug("error generating id", "error", err)
				return "", "", InternalServerError{}
			}
			baseDir = filepath.Join(home, "nsip_"+id)
			err = os.Mkdir(baseDir, 0755)
			if err != nil {
				return "", "", InternalServerError{}
			}

		}
	}
	_ = os.Mkdir(baseDir+"/proc", 0755)
	_ = os.Mkdir(baseDir+"/tmp", 0755)
	return baseDir, id, nil
}

func addSource(sourceName, baseDir, source string) error {
	err := os.WriteFile(fmt.Sprintf("%s/%s", baseDir, sourceName), []byte(source), 0755)
	if err != nil {
		slog.Debug("error creating and writing to file", "error", err)
		return InternalServerError{}
	}
	return nil
}

func createTestWS(baseDir string, tests []Tests) error {
	var err error
	for i := range tests {
		testDir := fmt.Sprintf("test_%d", i)
		err = os.Mkdir(fmt.Sprintf("%s/%s", baseDir, testDir), 0755)
		if err != nil {
			slog.Debug("error creating directory", "error", err)
			return InternalServerError{}
		}

		err = os.WriteFile(fmt.Sprintf("%s/%s/%s", baseDir, testDir, "input"), []byte(tests[i].Stdin), 0755)
		if err != nil {
			slog.Debug("error writing test case input", "error", err)
			return InternalServerError{}
		}
	}
	return nil
}

func compileCode(baseDir, binaryName, filename string,
	ipBuildOpts *LimitsAndFlags, languageCompilationOpts *Options,
	output *Response, defaultArgs string) error {
	slog.Debug("compiling program")
	args := []string{"--rw", "--log", baseDir + "/log",
		"-e", "--cwd", "/", "-c", baseDir}
	// TODO: override from input if available
	if ipBuildOpts.Limits.MaxProcesses != 0 {
		args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(ipBuildOpts.Limits.MaxProcesses), 10))
	} else if languageCompilationOpts.ResourceLimits.MaxProcesses != 0 {
		args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(languageCompilationOpts.ResourceLimits.MaxProcesses), 10))
	}
	if ipBuildOpts.Limits.MemoryKB != 0 {
		args = append(args, "--rlimit_as", strconv.FormatInt(ipBuildOpts.Limits.MemoryKB/1024, 10))
	} else if languageCompilationOpts.ResourceLimits.MemoryKB != 0 {
		args = append(args, "--rlimit_as", strconv.FormatInt(languageCompilationOpts.ResourceLimits.MemoryKB/1024, 10))
	}
	if ipBuildOpts.Limits.WallTime != 0 {
		args = append(args, "--time_limit", strconv.FormatInt(int64(ipBuildOpts.Limits.WallTime), 10))
	} else if languageCompilationOpts.ResourceLimits.WallTime != 0 {
		args = append(args, "--time_limit", strconv.FormatInt(int64(languageCompilationOpts.ResourceLimits.WallTime), 10))
	}
	args = append(args, strings.Fields(defaultArgs)...)

	compilation := strings.Join(languageCompilationOpts.Args, " ")
	compilation = strings.ReplaceAll(compilation, "{{flags}}",
		strings.Join(ipBuildOpts.Flags, " "))
	compilation = strings.ReplaceAll(compilation, "{{source}}", filename)
	compilation = strings.ReplaceAll(compilation, "{{artifact}}", binaryName)
	args = append(args, "--", languageCompilationOpts.Cmd)
	args = append(args, strings.Fields(compilation)...)
	cmd := exec.Command("/usr/local/bin/nsjail", args...)
	start := time.Now()
	out, err := cmd.CombinedOutput()
	log.Println(string(out))
	elapsed := time.Since(start).Milliseconds()
	output.Build = &ExecutionDetails{
		Status:   BuildOk,
		Duration: int(elapsed),
	}
	if err != nil {
		logs, _ := os.ReadFile(baseDir + "/log")
		output.Status = "compilation_error"
		output.Build.Status = BuildFailed
		output.Build.STDErr = string(logs)
		return nil
	}
	output.Build.STDOut = string(out)
	// filename = *binary

	return nil
}

func runCode(baseDir, id, defaultArgs, filename string, inputPref *LimitsAndFlags,
	langOpts Options, tests []Tests) []TestOutput {
	testResults := []TestOutput{}
	args := []string{"--rw", "-e", "--cwd", "/", "-c", baseDir}
	if inputPref != nil && inputPref.Limits.MaxProcesses != 0 {
		args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(inputPref.Limits.MaxProcesses), 10))
	} else if langOpts.ResourceLimits.MaxProcesses != 0 {
		args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(langOpts.ResourceLimits.MaxProcesses), 10))
	}
	if inputPref != nil && inputPref.Limits.WallTime != 0 {
		args = append(args, "--time_limit", strconv.FormatInt(int64(inputPref.Limits.WallTime), 10))
	} else if langOpts.ResourceLimits.WallTime != 0 {
		args = append(args, "--time_limit", strconv.FormatInt(int64(langOpts.ResourceLimits.WallTime), 10))
	}
	args = append(args, strings.Fields(defaultArgs)...)

	effectiveMemKB := langOpts.ResourceLimits.MemoryKB
	if inputPref != nil && inputPref.Limits.MemoryKB != 0 {
		effectiveMemKB = inputPref.Limits.MemoryKB
	}
	if effectiveMemKB > 0 {
		args = append(args, "--rlimit_as", strconv.FormatInt(effectiveMemKB/1024, 10))
	}

	path := langOpts.Cmd
	rtArgs := strings.Join(langOpts.Args, " ")
	rtArgs = strings.ReplaceAll(rtArgs, "{{source}}", filename)
	rtArgs = strings.ReplaceAll(rtArgs, "{{artifact}}", filename)
	args = append(args, "--", path)
	args = append(args, strings.Fields(rtArgs)...)

	for i := range tests {
		testDir := fmt.Sprintf("test_%d", i)
		ipFile := fmt.Sprintf("%s/%s/%s", baseDir, testDir, "input")

		ipFileCont, err := os.ReadFile(ipFile)
		if err != nil {
			testResults = append(testResults, TestOutput{
				ExecutionDetails: ExecutionDetails{
					Status:   "error",
					Duration: 0,
				},
			})
		}

		cgroupPath := fmt.Sprintf("/sys/fs/cgroup/goboxd/run_%s_%d", id, i)
		cgroupCreated := os.Mkdir(cgroupPath, 0755) == nil
		if cgroupCreated && effectiveMemKB > 0 {
			_ = os.WriteFile(cgroupPath+"/memory.max",
				[]byte(strconv.FormatInt(effectiveMemKB*1024, 10)), 0644)
		}

		cmd := exec.Command("/usr/local/bin/nsjail", args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Stdin = bytes.NewBuffer(ipFileCont)

		start := time.Now()
		runErr := cmd.Start()
		if cgroupCreated && cmd.Process != nil {
			_ = os.WriteFile(cgroupPath+"/cgroup.procs",
				[]byte(strconv.Itoa(cmd.Process.Pid)), 0644)
		}
		if runErr == nil {
			runErr = cmd.Wait()
		}
		elapsed := time.Since(start)
		out := stdout.Bytes()

		var memPeakKb int64
		if cgroupCreated {
			if raw, rerr := os.ReadFile(cgroupPath + "/memory.peak"); rerr == nil {
				if b, _ := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64); b > 0 {
					memPeakKb = b / 1024
				}
			}
			os.Remove(cgroupPath)
		}

		testRes := TestOutput{
			ExecutionDetails: ExecutionDetails{
				Status:   TestAccepted,
				STDOut:   string(out),
				Duration: int(elapsed.Milliseconds()),
			},
			MemoryPeakKb: memPeakKb,
		}
		if runErr != nil {
			stderrStr := stderr.String()
			slog.Debug("nsjail failed", "stderr", stderrStr, "args", cmd.Args)
			switch {
			case strings.Contains(stderrStr, "run time >= time limit"):
				testRes.Status = "time_exceeded"
			case effectiveMemKB > 0 && (strings.Contains(stderrStr, "MemoryError") ||
				strings.Contains(stderrStr, "terminated with signal: 9")):
				testRes.Status = "memory_exceeded"
			default:
				testRes.Status = "errored"
			}
			testRes.STDErr = stderrStr
		} else if string(out) != tests[i].ExpectedOut {
			testRes.ExecutionDetails.Status = "wrong_output"
		}
		testResults = append(testResults, testRes)
	}
	return testResults
}

func runEvaluationScript(baseDir, defaultArgs, sourceFilename, evalScriptLang string,
	ipConf *LimitsAndFlags, langOpts Options) *string {
	args := []string{"--rw", "-e", "--cwd", "/", "-c", baseDir}
	if ipConf != nil && ipConf.Limits.MaxProcesses != 0 {
		args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(ipConf.Limits.MaxProcesses), 10))
	} else if langOpts.ResourceLimits.MaxProcesses != 0 {
		args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(langOpts.ResourceLimits.MaxProcesses), 10))
	}
	if ipConf != nil && ipConf.Limits.MemoryKB != 0 {
		args = append(args, "--rlimit_as", strconv.FormatInt(ipConf.Limits.MemoryKB, 10))
	} else if langOpts.ResourceLimits.MemoryKB != 0 {
		args = append(args, "--rlimit_as", strconv.FormatInt(langOpts.ResourceLimits.MemoryKB, 10))
	}
	if ipConf != nil && ipConf.Limits.WallTime != 0 {
		args = append(args, "--time_limit", strconv.FormatInt(int64(ipConf.Limits.WallTime), 10))
	} else if langOpts.ResourceLimits.WallTime != 0 {
		args = append(args, "--time_limit", strconv.FormatInt(int64(langOpts.ResourceLimits.WallTime), 10))
	}
	args = append(args, strings.Fields(defaultArgs)...)

	args = append(args, "--", evalScriptLang, "evaluator.script", sourceFilename)

	cmd := exec.Command("/usr/local/bin/nsjail", args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		slog.Debug("evaluation script failed", "stderr", err.Error(), "args", cmd.Args)
	}
	result := stdout.String()
	return &result
}
