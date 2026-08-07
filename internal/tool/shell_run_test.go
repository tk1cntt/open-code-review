// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"strings"
	"testing"
)

func TestShellRun_Echo(t *testing.T) {
	dir := t.TempDir()
	p := NewShellRun(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"command": "echo hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "hello") {
		t.Fatalf("expected 'hello' in output, got: %s", result)
	}
	if !strings.Contains(result, "Exit code: 0") {
		t.Fatalf("expected Exit code: 0, got: %s", result)
	}
}

func TestShellRun_GoVersion(t *testing.T) {
	dir := t.TempDir()
	p := NewShellRun(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"command": "go version",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "go version") {
		t.Fatalf("expected 'go version' in output, got: %s", result)
	}
	if !strings.Contains(result, "Exit code: 0") {
		t.Fatalf("expected Exit code: 0, got: %s", result)
	}
}

func TestShellRun_EmptyCommand(t *testing.T) {
	dir := t.TempDir()
	p := NewShellRun(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"command": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "command is required") {
		t.Fatalf("expected command required error, got: %s", result)
	}
}

func TestShellRun_BlockedMetacharacters(t *testing.T) {
	dir := t.TempDir()
	p := NewShellRun(dir)

	tests := []string{"echo a; echo b", "echo a && echo b", "echo a || echo b", "echo a | echo b", "echo `id`", "echo $(id)", "echo ${HOME}", "echo a > b", "echo a >> b", "echo a < b"}
	for _, cmd := range tests {
		result, err := p.Execute(context.Background(), map[string]any{
			"command": cmd,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result, "blocked shell metacharacter") {
			t.Fatalf("expected metacharacter block for %q, got: %s", cmd, result)
		}
	}
}

func TestShellRun_NotAllowedCommand(t *testing.T) {
	dir := t.TempDir()
	p := NewShellRun(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"command": "rm -rf /",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "not in the allowed list") {
		t.Fatalf("expected 'not in the allowed list' error, got: %s", result)
	}
}

func TestShellRun_CommandFailed(t *testing.T) {
	dir := t.TempDir()
	p := NewShellRun(dir)
	result, err := p.Execute(context.Background(), map[string]any{
		"command": "go build ./nonexistent_dir",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "Command failed") && !strings.Contains(result, "Exit code:") {
		t.Fatalf("expected failure indication, got: %s", result)
	}
}

func TestShellRun_CustomTimeout(t *testing.T) {
	dir := t.TempDir()
	p := NewShellRun(dir)
	// Use a command that should complete very quickly with a short timeout.
	result, err := p.Execute(context.Background(), map[string]any{
		"command":     "echo quick",
		"timeout_sec": float64(5),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "quick") {
		t.Fatalf("expected 'quick' in output, got: %s", result)
	}
}
