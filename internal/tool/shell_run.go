// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	shellRunDefaultTimeout = 30 * time.Second
	shellRunMaxTimeout     = 60 * time.Second
	shellRunMaxOutputBytes = 100 * 1024 // 100 KB output limit
)

// ShellRunProvider executes safe, whitelisted shell commands within the
// repository directory. It applies strict security checks: only commands
// starting with an allowed prefix are accepted, and shell metacharacters
// used for chaining or redirection are rejected.
type ShellRunProvider struct {
	RepoDir string

	// Allowlist of allowed command prefixes (e.g. "go ", "npm ", "python ").
	// A command must start with one of these prefixes.
	allowList []string
}

// defaultShellAllowList is the set of allowed command prefixes when no custom
// allowlist is provided.
var defaultShellAllowList = []string{
	"go ", "go.",
	"npm ", "npx ",
	"yarn ",
	"pnpm ",
	"node ",
	"python ", "python3 ",
	"pip ",
	"cargo ",
	"rustc ",
	"make ", "cmake ",
	"gcc ", "g++", "clang ", "clang++",
	"javac ", "java ",
	"dotnet ",
	"echo ",
	"ls ", "dir ",
	"cat ",
	"grep ",
	"head ", "tail ",
	"wc ",
	"find ",
	"git status", "git diff", "git log", "git branch",
	"pwd",
	"env",
	"uname",
}

// NewShellRun creates a ShellRunProvider with a default allowlist.
func NewShellRun(repoDir string) *ShellRunProvider {
	return &ShellRunProvider{
		RepoDir:   repoDir,
		allowList: defaultShellAllowList,
	}
}

func (p *ShellRunProvider) Tool() Tool { return ShellRun }

func (p *ShellRunProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	cmd, _ := args["command"].(string)
	if cmd == "" {
		return "Error: command is required", nil
	}

	cmd = strings.TrimSpace(cmd)

	// Security check 1: block shell metacharacters used for chaining or redirection.
	if err := p.validateMetacharacters(cmd); err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	// Security check 2: command must match an allowed prefix.
	if !p.isAllowed(cmd) {
		return fmt.Sprintf(
			"Error: command %q is not in the allowed list. "+
				"Allowed commands include: go build, go test, npm test, cargo check, python, etc. "+
				"Use single, non-destructive build/test/lint commands only.", cmd), nil
	}

	// Resolve timeout.
	timeoutSec := 30
	if v, ok := args["timeout_sec"].(float64); ok && v > 0 {
		timeoutSec = int(v)
		if timeoutSec > 60 {
			timeoutSec = 60
		}
	}
	timeout := time.Duration(timeoutSec) * time.Second

	// Create context with timeout.
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Parse command into argv.
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "Error: empty command", nil
	}

	// Run the command.
	var exe *exec.Cmd
	if len(parts) == 1 {
		exe = exec.CommandContext(execCtx, parts[0])
	} else {
		exe = exec.CommandContext(execCtx, parts[0], parts[1:]...)
	}
	exe.Dir = p.RepoDir

	output, err := exe.CombinedOutput()

	// Truncate output if too large.
	if len(output) > shellRunMaxOutputBytes {
		output = output[:shellRunMaxOutputBytes]
		output = append(output, []byte(fmt.Sprintf(
			"\n... (output truncated at %d bytes)", shellRunMaxOutputBytes))...)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Command: %s\n", cmd))
	sb.WriteString(fmt.Sprintf("Exit code: %d\n", exe.ProcessState.ExitCode()))
	sb.WriteString(fmt.Sprintf("Stdout+Stderr:\n%s", string(output)))

	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			sb.WriteString(fmt.Sprintf(
				"\n\nError: command timed out after %d seconds.", timeoutSec))
		} else {
			sb.WriteString(fmt.Sprintf("\n\nCommand failed: %v", err))
		}
	}

	return sb.String(), nil
}

// validateMetacharacters rejects commands containing shell metacharacters
// that can be used for command chaining, piping, redirection or substitution.
func (p *ShellRunProvider) validateMetacharacters(cmd string) error {
	blocked := []string{
		";", "&&", "||", "|", "`", "$(", "${", ">", ">>", "<", "<<", "&",
	}
	for _, meta := range blocked {
		if strings.Contains(cmd, meta) {
			return fmt.Errorf(
				"command contains blocked shell metacharacter %q. "+
					"Only single, safe commands are allowed.", meta)
		}
	}
	return nil
}

// isAllowed checks whether cmd starts with any prefix in the allowlist.
func (p *ShellRunProvider) isAllowed(cmd string) bool {
	for _, prefix := range p.allowList {
		if strings.HasPrefix(cmd, prefix) {
			return true
		}
	}
	return false
}
