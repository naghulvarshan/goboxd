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
	output := types.Response{}
	if languageConfig.CompilationOpts != nil {
		slog.Debug("compiling program")
		args := []string{"--rw", "--log", baseDir + "/log",
			"-e", "--cwd", "/", "-c", baseDir}
		// TODO: override from input if available
		if input.Build.Limits.MaxProcesses != 0 {
			args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(input.Build.Limits.MaxProcesses), 10))
		}
		if input.Build.Limits.MemoryKB != 0 {
			args = append(args, "--rlimit_as", strconv.FormatInt(input.Build.Limits.MemoryKB, 10))
		}
		if input.Build.Limits.WallTime != 0 {
			args = append(args, "--time_limit", strconv.FormatInt(int64(input.Build.Limits.WallTime), 10))
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
		output.Build = types.ExecutionDetails{
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
	if input.Run.Limits.MaxProcesses != 0 {
		args = append(args, "--rlimit_nproc", strconv.FormatInt(int64(input.Build.Limits.MaxProcesses), 10))
	}
	if input.Run.Limits.MemoryKB != 0 {
		args = append(args, "--rlimit_as", strconv.FormatInt(input.Build.Limits.MemoryKB, 10))
	}
	if input.Run.Limits.WallTime != 0 {
		args = append(args, "--time_limit", strconv.FormatInt(int64(input.Build.Limits.WallTime), 10))
	}
	args = append(args, strings.Fields(defaultArgs)...)
	path := languageConfig.RuntimeOpts.Path
	rtArgs := languageConfig.RuntimeOpts.Args
	rtArgs = strings.ReplaceAll(rtArgs, "{{ FILENAME }}", filename)
	args = append(args, path)
	args = append(args, strings.Fields(rtArgs)...)
	for _ = range input.Tests {
		cmd := exec.Command("/usr/local/bin/nsjail", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		//TODO: Add input stream when required
		start := time.Now()
		out, err := cmd.Output()
		elapsed := time.Since(start)
		//TODO: Validate test outputs with expected outputs
		testRes := types.TestOutput{ExecutionDetails: types.ExecutionDetails{
			Status:   "success",
			STDOut:   string(out),
			Duration: int(elapsed.Milliseconds()),
		}}
		if err != nil {
			slog.Debug("nsjail failed",
				"stderr", stderr.String(),
				"args", cmd.Args,
			)
			testRes.Status = "errored"
			testRes.STDErr = stderr.String()
		}

		output.TestOutputs = append(output.TestOutputs, testRes)
		log.Println(string(out))
	}
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
