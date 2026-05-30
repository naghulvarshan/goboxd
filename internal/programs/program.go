package programs

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

	"github.com/thesouldev/goboxd/internal/types"
)

func Run(input *types.ProgramInfo, defaultArgs string, languageConfig types.LanguageSettings) (*types.Response, error) {

	// Step 1: Initialize a directory to execute the code

	id, err := idGenerator()
	if err != nil {
		slog.Debug("error generating id", "error", err)
		return nil, errors.New("internal server error")
	}
	home, _ := os.UserHomeDir()
	baseDir := filepath.Join(home, "nsip_"+id)
	err = os.Mkdir(baseDir, 0755)
	if err != nil {
		if strings.Contains(err.Error(), "File exists") { // Retry once if a duplicate id is created
			slog.Debug("directory already exists, retrying...", "directory", baseDir)
			id, err = idGenerator()
			if err != nil {
				slog.Debug("error generating id", "error", err)
				return nil, errors.New("internal server error")
			}
			baseDir = filepath.Join(home, "nsip_"+id)
			err = os.Mkdir(baseDir, 0755)
			if err != nil {
				return nil, errors.New("internal server error")
			}

		}
	}
	os.Mkdir(baseDir+"/proc", 0755)

	// Step 2: Write the program to a file
	filename := languageConfig.FileName
	if input.SourceFileName != nil {
		filename = *input.SourceFileName
	}
	err = os.WriteFile(fmt.Sprintf("%s/%s", baseDir, filename), []byte(input.Source), 0755)
	if err != nil {
		slog.Debug("error creating and writing to file", "error", err)
		return nil, errors.New("internal server error")
	}

	// Step 3: Create test directories
	for i := range input.Tests {
		testDir := fmt.Sprintf("test_%d", i)
		err = os.Mkdir(fmt.Sprintf("%s/%s", baseDir, testDir), 0755)
		if err != nil {
			slog.Debug("error creating directory", "error", err)
			return nil, errors.New("internal server error")
		}

		err = os.WriteFile(fmt.Sprintf("%s/%s/%s", baseDir, testDir, "input"), []byte(input.Tests[i].Stdin), 0755)
		if err != nil {
			slog.Debug("error writing test case input", "error", err)
			return nil, errors.New("internal server error")
		}
	}

	// Step 4: Compile if needed
	output := types.Response{
		Status: "success",
	}
	if languageConfig.CompilationOpts != nil {
		slog.Debug("compiling program")
		args := []string{"--rw", "--log", baseDir + "/log",
			"-e", "--cwd", "/", "-c", baseDir}
		// TODO: override from input if available
		if input.Build.Limits.MaxProcesses != 0 {
			args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(input.Build.Limits.MaxProcesses), 10))
		} else if languageConfig.CompilationOpts.ResourceLimits.MaxProcesses != 0 {
			args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(languageConfig.CompilationOpts.ResourceLimits.MaxProcesses), 10))
		}
		if input.Build.Limits.MemoryKB != 0 {
			args = append(args, "--rlimit_as", strconv.FormatInt(input.Build.Limits.MemoryKB, 10))
		} else if languageConfig.CompilationOpts.ResourceLimits.MemoryKB != 0 {
			args = append(args, "--rlimit_as", strconv.FormatInt(languageConfig.CompilationOpts.ResourceLimits.MemoryKB, 10))
		}
		if input.Build.Limits.WallTime != 0 {
			args = append(args, "--time_limit", strconv.FormatInt(int64(input.Build.Limits.WallTime), 10))
		} else if languageConfig.CompilationOpts.ResourceLimits.WallTime != 0 {
			args = append(args, "--time_limit", strconv.FormatInt(int64(languageConfig.CompilationOpts.ResourceLimits.WallTime), 10))
		}
		args = append(args, strings.Fields(defaultArgs)...)
		fmt.Printf("defaultArgs: %q\n", defaultArgs)
		fmt.Printf("after split: %q\n", strings.Fields(defaultArgs))
		args = append(args, "--symlink", "/lib:/lib64") // To be removed
		binary := languageConfig.BinaryFileName
		if binary == nil {
			binary = input.ArtifaceFileName
		}
		if binary == nil {
			return nil, errors.New("binary artifact name is required")
		}
		compilation := languageConfig.CompilationOpts.Args
		compilation = strings.ReplaceAll(compilation, "{{ EXTRA_ARGS }}",
			strings.Join(input.Build.Flags, " "))
		compilation = strings.ReplaceAll(compilation, "{{ FILENAME }}", filename)
		args = append(args, "--", languageConfig.CompilationOpts.Path)
		args = append(args, strings.Fields(compilation)...)
		fmt.Printf("full args: %q\n", args)
		cmd := exec.Command("/usr/local/bin/nsjail", args...)
		start := time.Now()
		out, err := cmd.CombinedOutput()
		log.Println(string(out))
		elapsed := time.Since(start).Milliseconds()
		output.Build = &types.ExecutionDetails{
			Status:   "success",
			Duration: int(elapsed),
		}
		if err != nil {
			logs, _ := os.ReadFile(baseDir + "/log")
			output.Status = "compilation_error"
			output.Build.Status = "error"
			output.Build.STDErr = string(logs)
			log.Println(err)
			return &output, nil
		}
		output.Build.STDOut = string(out)
		// filename = *binary
	}

	// Step 5: Running code
	args := []string{"--rw", "-e", "--cwd", "/", "-c", baseDir}
	if input.Run != nil && input.Run.Limits.MaxProcesses != 0 {
		args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(input.Run.Limits.MaxProcesses), 10))
	} else if languageConfig.RuntimeOpts.ResourceLimits.MaxProcesses != 0 {
		args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(languageConfig.CompilationOpts.ResourceLimits.MaxProcesses), 10))
	}
	if input.Run != nil && input.Run.Limits.MemoryKB != 0 {
		args = append(args, "--rlimit_as", strconv.FormatInt(input.Run.Limits.MemoryKB, 10))
	} else if languageConfig.CompilationOpts.ResourceLimits.MemoryKB != 0 {
		args = append(args, "--rlimit_as", strconv.FormatInt(languageConfig.CompilationOpts.ResourceLimits.MemoryKB, 10))
	}
	if input.Run != nil && input.Run.Limits.WallTime != 0 {
		args = append(args, "--time_limit", strconv.FormatInt(int64(input.Run.Limits.WallTime), 10))
	} else if languageConfig.CompilationOpts.ResourceLimits.WallTime != 0 {
		args = append(args, "--time_limit", strconv.FormatInt(int64(languageConfig.CompilationOpts.ResourceLimits.WallTime), 10))
	}
	args = append(args, strings.Fields(defaultArgs)...)
	path := languageConfig.RuntimeOpts.Path
	rtArgs := languageConfig.RuntimeOpts.Args
	rtArgs = strings.ReplaceAll(rtArgs, "{{ FILENAME }}", filename)
	binaryFilename := filename
	if languageConfig.BinaryFileName != nil && *languageConfig.BinaryFileName != "TAKE_FROM_REQUEST" {
		binaryFilename = *languageConfig.BinaryFileName
	} else if input.ArtifaceFileName != nil {
		binaryFilename = *input.ArtifaceFileName
	}
	rtArgs = strings.ReplaceAll(rtArgs, "{{ BINARY_FILENAME }}", binaryFilename)
	args = append(args, path)
	args = append(args, strings.Fields(rtArgs)...)
	for i := range input.Tests {
		testDir := fmt.Sprintf("test_%d", i)
		ipFile := fmt.Sprintf("%s/%s/%s", baseDir, testDir, "input")

		ipFileCont, err := os.ReadFile(ipFile)
		if err != nil {
			output.TestOutputs = append(output.TestOutputs, types.TestOutput{
				ExecutionDetails: types.ExecutionDetails{
					Status:   "error",
					Duration: 0,
				},
			})
		}

		cgroupPath := fmt.Sprintf("/sys/fs/cgroup/goboxd/run_%s_%d", id, i)
		cgroupCreated := os.Mkdir(cgroupPath, 0755) == nil

		cmd := exec.Command("/usr/local/bin/nsjail", args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		cmd.Stdin = bytes.NewBuffer(ipFileCont)

		start := time.Now()
		runErr := cmd.Start()
		if cgroupCreated && cmd.Process != nil {
			os.WriteFile(cgroupPath+"/cgroup.procs",
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

		testRes := types.TestOutput{
			ExecutionDetails: types.ExecutionDetails{
				Status:   "success",
				STDOut:   string(out),
				Duration: int(elapsed.Milliseconds()),
			},
			MemoryPeakKb: memPeakKb,
		}
		if runErr != nil {
			slog.Debug("nsjail failed",
				"stderr", stderr.String(),
				"args", cmd.Args,
			)
			testRes.Status = "errored"
			testRes.STDErr = stderr.String()
		} else if string(out) != input.Tests[i].ExpectedOut {
			testRes.ExecutionDetails.Status = "wrong_output"
		}
		output.TestOutputs = append(output.TestOutputs, testRes)
		log.Println(string(out))
	}
	os.RemoveAll(baseDir)
	return &output, nil
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
