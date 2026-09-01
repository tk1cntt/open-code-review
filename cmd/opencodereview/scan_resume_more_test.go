// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
)

// writeScanResumeSession persists a full-scan session with the given completed
// file checkpoints and returns its ID. HOME must already point at a temp dir.
// Comments must be non-empty: zero-comment review_item_done records are treated
// as incomplete and land in FailedFiles rather than completed Items.
func writeScanResumeSession(t *testing.T, repoDir string, files ...string) string {
	t.Helper()
	sh := session.New(repoDir, "feature", "fake", session.SessionOptions{
		ReviewMode: session.ReviewModeFullScan,
	})
	for _, f := range files {
		sh.RecordReviewItemDone(f, "", f, "fp-"+f, []model.LlmComment{{
			Path: f, Content: "ok", StartLine: 1, EndLine: 1,
		}})
	}
	if err := sh.Finalize(); err != nil {
		t.Fatalf("finalize session: %v", err)
	}
	return sh.SessionID
}

// TestLoadScanResumeState_WithSession drives the fixture-backed branches of
// loadScanResumeState: a successful resume, a scan-mode mismatch, and a session
// that completed no items.
func TestLoadScanResumeState_WithSession(t *testing.T) {
	t.Run("success returns state with completed items", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeScanResumeSession(t, repoDir, "a.go", "b.go")

		state, err := loadScanResumeState(repoDir, scanOptions{resume: id})
		if err != nil {
			t.Fatalf("loadScanResumeState: %v", err)
		}
		if state == nil || state.CompletedCount() != 2 {
			t.Fatalf("got %v, want state with 2 completed items", state)
		}
	})

	t.Run("non-scan session rejected", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		// Persist a range-mode session, then try to resume it as a scan.
		id := writeRangeResumeSession(t, repoDir, "a.go")

		_, err := loadScanResumeState(repoDir, scanOptions{resume: id})
		if err == nil {
			t.Fatal("expected error resuming a non-scan session")
		}
	})

	t.Run("empty session still loads for fresh scan", func(t *testing.T) {
		setTestHome(t, t.TempDir())
		repoDir := t.TempDir()
		id := writeScanResumeSession(t, repoDir) // no items recorded

		state, err := loadScanResumeState(repoDir, scanOptions{resume: id})
		if err != nil {
			t.Fatalf("loadScanResumeState: %v", err)
		}
		if state == nil {
			t.Fatal("expected non-nil resume state")
		}
		if state.HasSessionScope() {
			t.Fatal("empty session should have no session scope")
		}
	})
}
