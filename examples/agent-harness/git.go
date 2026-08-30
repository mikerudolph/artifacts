package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func runGit(ctx context.Context, dir, credential string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if credential != "" {
		secret, _, _ := strings.Cut(credential, "?")
		header := "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte("agent:"+secret))
		cmd.Env = append(cmd.Env,
			"GIT_CONFIG_COUNT=2",
			"GIT_CONFIG_KEY_0=protocol.version", "GIT_CONFIG_VALUE_0=1",
			"GIT_CONFIG_KEY_1=http.extraHeader", "GIT_CONFIG_VALUE_1="+header,
		)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}
