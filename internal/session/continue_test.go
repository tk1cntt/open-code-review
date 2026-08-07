package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

func TestPrepareFileStart_Modes(t *testing.T) {
	resume := &ResumeState{
		SessionID: "s1",
		Items: map[string]ResumeItem{
			"fp-done": {Fingerprint: "fp-done", FilePath: "done.go", Comments: []model.LlmComment{{Path: "done.go", Content: "c"}}},
		},
		FailedFiles: map[string]string{"fp-fail": "fail.go"},
		Conversations: map[string]ConversationCheckpoint{
			"fp-mid": {
				Fingerprint: "fp-mid",
				FilePath:    "mid.go",
				Messages:    []llm.Message{llm.NewTextMessage("user", "hello")},
				Round:       2,
				Model:       "m1",
				TemplateHash: "th1",
				Status:      CheckpointTimedOut,
				PlanGuidance: "plan-x",
				Comments:    []model.LlmComment{{Path: "mid.go", Content: "partial"}},
			},
		},
	}

	// Reuse
	got := PrepareFileStart(resume, "fp-done", "done.go", PrepareOpts{})
	if got.Mode != ModeReuse {
		t.Fatalf("done mode = %q, want reuse", got.Mode)
	}

	// Continue
	got = PrepareFileStart(resume, "fp-mid", "mid.go", PrepareOpts{CurrentModel: "m1", TemplateHash: "th1"})
	if got.Mode != ModeContinue {
		t.Fatalf("mid mode = %q, want continue", got.Mode)
	}
	if got.Round != 2 || got.PlanGuidance != "plan-x" || len(got.Messages) != 1 {
		t.Fatalf("continue payload unexpected: %+v", got)
	}

	// Model mismatch → cold (with seed comments)
	got = PrepareFileStart(resume, "fp-mid", "mid.go", PrepareOpts{CurrentModel: "other", TemplateHash: "th1"})
	if got.Mode != ModeCold {
		t.Fatalf("model mismatch mode = %q, want cold", got.Mode)
	}
	if len(got.SeedComments) != 1 {
		t.Fatalf("expected seed comments on invalid continue, got %d", len(got.SeedComments))
	}

	// restart-failed ignores checkpoint
	got = PrepareFileStart(resume, "fp-mid", "mid.go", PrepareOpts{
		ResumeMode:   ResumeModeRestartFailed,
		CurrentModel: "m1",
		TemplateHash: "th1",
	})
	if got.Mode != ModeCold {
		t.Fatalf("restart-failed mode = %q, want cold", got.Mode)
	}

	// Failed without checkpoint → cold
	got = PrepareFileStart(resume, "fp-fail", "fail.go", PrepareOpts{})
	if got.Mode != ModeCold {
		t.Fatalf("failed cold mode = %q", got.Mode)
	}
}

func TestSaveAndLoadConversationCheckpoint(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := filepath.Join(t.TempDir(), "repo")
	sh := New(repoDir, "main", "model-a", SessionOptions{ReviewMode: ReviewModeFullScan})
	fp := "fp-operation-queue"
	msgs := []llm.Message{
		llm.NewTextMessage("system", "sys"),
		llm.NewTextMessage("user", "review file"),
		llm.NewToolCallMessage("thinking", []llm.ToolCall{{
			ID: "c1", Type: "function",
			Function: llm.FunctionCall{Name: "file_find", Arguments: `{"query_name":"log-helper"}`},
		}}),
		llm.NewToolResultMessage("c1", "found log-helper.ts"),
	}
	sh.SaveConversationCheckpoint(ConversationCheckpoint{
		FilePath:     "operation-queue.ts",
		Fingerprint:  fp,
		Phase:        PhaseMain,
		PlanGuidance: "focus on races",
		Messages:     msgs,
		Round:        2,
		Model:        "model-a",
		TemplateHash: "tmpl1",
		Status:       CheckpointInProgress,
		Comments:     []model.LlmComment{{Path: "operation-queue.ts", Content: "race", StartLine: 10, EndLine: 12}},
	})
	sh.MarkCheckpointStopped(fp, CheckpointTimedOut, "timeout")
	sh.RecordReviewItemFailed("operation-queue.ts", "operation-queue.ts", "operation-queue.ts", fp, "timeout", nil)
	if err := sh.Finalize(); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Live map still has checkpoint
	if _, ok := sh.LastConversationCheckpoint(fp); !ok {
		t.Fatal("expected live checkpoint after fail")
	}

	state, err := LoadResumeState(repoDir, sh.SessionID)
	if err != nil {
		t.Fatalf("LoadResumeState: %v", err)
	}
	cp, ok := state.Conversation(fp)
	if !ok {
		t.Fatal("expected conversation checkpoint in resume state")
	}
	if cp.Round != 2 || cp.Status != CheckpointTimedOut {
		t.Fatalf("checkpoint = %+v", cp)
	}
	if len(cp.Messages) != 4 {
		t.Fatalf("messages = %d, want 4", len(cp.Messages))
	}
	// Ensure tool result is preserved so resume won't re-run file_find.
	found := false
	for _, m := range cp.Messages {
		if m.Role == "tool" && m.ExtractText() == "found log-helper.ts" {
			found = true
		}
	}
	if !found {
		t.Fatal("tool result missing from checkpoint messages")
	}

	start := PrepareFileStart(state, fp, "operation-queue.ts", PrepareOpts{
		CurrentModel: "model-a",
		TemplateHash: "tmpl1",
	})
	if start.Mode != ModeContinue {
		t.Fatalf("mode = %q, want continue", start.Mode)
	}
}

func TestCheckpointClearedOnDone(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := filepath.Join(t.TempDir(), "repo")
	sh := New(repoDir, "main", "m", SessionOptions{ReviewMode: ReviewModeFullScan})
	fp := "fp1"
	sh.SaveConversationCheckpoint(ConversationCheckpoint{
		FilePath: "a.go", Fingerprint: fp, Messages: []llm.Message{llm.NewTextMessage("user", "x")}, Round: 1,
	})
	sh.RecordReviewItemDone("a.go", "a.go", "a.go", fp, []model.LlmComment{{Path: "a.go", Content: "ok", StartLine: 1, EndLine: 1}})
	if _, ok := sh.LastConversationCheckpoint(fp); ok {
		t.Fatal("checkpoint should clear on done")
	}
	if err := sh.Finalize(); err != nil {
		t.Fatal(err)
	}
	state, err := LoadResumeState(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.Conversation(fp); ok {
		t.Fatal("done item must not leave conversation checkpoint")
	}
	if state.CompletedCount() != 1 {
		t.Fatalf("completed = %d", state.CompletedCount())
	}
}

func TestLLMRequestFlush_DurableBeforeCrash(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := filepath.Join(t.TempDir(), "repo")
	sh := New(repoDir, "main", "m", SessionOptions{ReviewMode: ReviewModeFullScan})
	fs := sh.GetOrCreateFileSession("a.go")
	_ = fs.AppendTaskRecord(MainTask, []llm.Message{llm.NewTextMessage("user", "hi")})

	// Without Finalize, flush policy must still have written llm_request to disk.
	path, err := SessionFilePath(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sawRequest bool
	for _, line := range splitLines(data) {
		var rec map[string]any
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if rec["type"] == "llm_request" {
			sawRequest = true
		}
	}
	if !sawRequest {
		t.Fatal("llm_request must be flushed to disk immediately")
	}
	// Close the writer so TempDir cleanup can remove the JSONL on Windows.
	_ = sh.Finalize()
}

func TestCommentFingerprintStable(t *testing.T) {
	a := model.LlmComment{Path: "x.go", Content: "bug", StartLine: 1, EndLine: 2}
	b := model.LlmComment{Path: "x.go", Content: "bug", StartLine: 99, EndLine: 100}
	if CommentFingerprint(a) != CommentFingerprint(b) {
		t.Fatal("content-based fingerprint should ignore line numbers")
	}
}

func splitLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			out = append(out, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}

func TestFormatResumeSummary(t *testing.T) {
	s := FormatResumeSummary("abc", 1, 2, 3, 4)
	if s == "" || s[0] != '[' {
		t.Fatalf("unexpected summary: %q", s)
	}
}

func TestResumeState_InSessionScope(t *testing.T) {
	s := &ResumeState{
		Items: map[string]ResumeItem{
			"fp-done": {Fingerprint: "fp-done", FilePath: "done.go"},
		},
		FailedFiles: map[string]string{"fp-fail": "fail.go"},
		Conversations: map[string]ConversationCheckpoint{
			"fp-mid": {Fingerprint: "fp-mid", FilePath: "mid.go"},
		},
	}
	if !s.HasSessionScope() {
		t.Fatal("expected session scope")
	}
	if !s.InSession("fp-done") || !s.InSession("fp-fail") || !s.InSession("fp-mid") {
		t.Fatal("expected all known fingerprints InSession")
	}
	if s.InSession("fp-other") {
		t.Fatal("unknown file must not be InSession")
	}
	if s.NeedsDispatch("fp-done") {
		t.Fatal("done file must not need dispatch")
	}
	if !s.NeedsDispatch("fp-fail") || !s.NeedsDispatch("fp-mid") {
		t.Fatal("failed/mid must need dispatch")
	}
	if (&ResumeState{}).HasSessionScope() {
		t.Fatal("empty state has no scope")
	}
}
