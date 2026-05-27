package programs

import (
	"bytes"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/thesouldev/goboxd/internal/types"
)

func Run(input *types.ProgramInfo, defaultArgs string, languageConfig types.LanguageSettings) (*types.Response, error) {
	//TODO: implement code compiling
	resp := &types.Response{Status: "success"}
	for t := range input.Tests {
		dir := fmt.Sprintf("test-%d", t)
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return nil, err
		}
		err = os.WriteFile(dir+"/"+*input.SourceFileName, []byte(input.Source), os.ModePerm)
		if err != nil {
			return nil, err
		}
		args := strings.Fields(defaultArgs)
		cmdArgs := []string{"-Mo", "--chroot", dir}
		cmdArgs = append(cmdArgs, args...)
		cmdArgs = append(cmdArgs, "--", languageConfig.RuntimeOpts.Path, *input.SourceFileName)

		cmd := exec.Command("/usr/local/bin/nsjail", cmdArgs...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		//TODO: Add input stream when required
		out, err := cmd.Output()
		//TODO: Validate test outputs with expected outputs
		output := types.TestOutput{ExecutionDetails: types.ExecutionDetails{
			Status: "success",
			STDOut: string(out),
		}}
		if err != nil {
			slog.Debug("nsjail failed",
				"stderr", stderr.String(),
				"args", cmd.Args,
			)
			output.Status = "errored"
			output.STDErr = stderr.String()
		}

		resp.TestOutputs = append(resp.TestOutputs, output)
		log.Println(string(out))
	}
	return resp, nil
}
