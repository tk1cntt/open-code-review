package llmloop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func init() { session.UseTestSessions() }

// scriptedClient returns scripted responses; after scripts are exhausted it
// returns a timeout error (simulating deadline mid-file).
type scriptedClient struct {
	responses []*llm.ChatResponse
	calls     atomic.Int64
	failAfter int // if >= 0, fail on that 0-based call index with timeout
}

func (s *scriptedClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	n := int(s.calls.Add(1) - 1)
	if s.failAfter >= 0 && n == s.failAfter {
		return nil, context.DeadlineExceeded
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if n >= len(s.responses) {
		return nil, errors.New("no more scripted responses")
	}
	return s.responses[n], nil
}

type countingFileFind struct {
	calls atomic.Int64
}

func (c *countingFileFind) Tool() tool.Tool { return tool.FileFind }
func (c *countingFileFind) Execute(_ context.Context, _ map[string]any) (string, error) {
	c.calls.Add(1)
	return "matches: log-helper.ts", nil
}

type countingFileRead struct {
	calls atomic.Int64
}

func (c *countingFileRead) Tool() tool.Tool { return tool.FileRead }
func (c *countingFileRead) Execute(_ context.Context, _ map[string]any) (string, error) {
	c.calls.Add(1)
	return "export function helper() {}", nil
}

func toolCallResp(id, name, args string) *llm.ChatResponse {
	content := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{
			Message: llm.ResponseMessage{
				Content: &content,
				ToolCalls: []llm.ToolCall{{
					ID: id, Type: "function",
					Function: llm.FunctionCall{Name: name, Arguments: args},
				}},
			},
		}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 10, CompletionTokens: 5},
	}
}

func TestE2E_MidFileCheckpointResume_NoToolReplay(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := filepath.Join(t.TempDir(), "repo")
	path := "operation-queue.ts"
	fingerprint := "fp-op-queue"

	find := &countingFileFind{}
	read := &countingFileRead{}
	reg := tool.NewRegistry()
	reg.Register(find)
	reg.Register(read)

	// Round 1: file_find, Round 2: file_read, Round 3: timeout before task_done.
	client1 := &scriptedClient{
		failAfter: 2,
		responses: []*llm.ChatResponse{
			toolCallResp("c1", "file_find", `{"query_name":"log-helper"}`),
			toolCallResp("c2", "file_read", `{"file_path":"log-helper.ts","start_line":1,"end_line":30}`),
		},
	}
	sh1 := session.New(repoDir, "main", "fake-model", session.SessionOptions{ReviewMode: session.ReviewModeFullScan})
	collector := tool.NewCommentCollector()
	r1 := NewRunner(Deps{
		LLMClient:        client1,
		Model:            "fake-model",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: 10},
		Tools:            reg,
		CommentCollector: collector,
		Session:          sh1,
	})
	BindSessionCheckpoint(r1, sh1, fingerprint, "plan-guidance", "fake-model", "tmpl-hash")

	msgs := []llm.Message{
		llm.NewTextMessage("system", "refactor this file"),
		llm.NewTextMessage("user", "start"),
	}
	completed, _, err := r1.RunPerFile(context.Background(), msgs, path)
	if completed {
		t.Fatal("expected incomplete run")
	}
	if err == nil || !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("expected deadline error, got %v", err)
	}
	if find.calls.Load() != 1 || read.calls.Load() != 1 {
		t.Fatalf("tools find=%d read=%d, want 1 each", find.calls.Load(), read.calls.Load())
	}
	cp, ok := sh1.LastConversationCheckpoint(fingerprint)
	if !ok || cp.Round != 2 {
		t.Fatalf("checkpoint after timeout: ok=%v round=%d", ok, cp.Round)
	}
	sh1.MarkCheckpointStopped(fingerprint, session.CheckpointTimedOut, err.Error())
	sh1.RecordReviewItemFailed(path, path, path, fingerprint, err.Error(), collector.CommentsForPath(path))
	sessionID := sh1.SessionID
	if ferr := sh1.Finalize(); ferr != nil {
		t.Fatalf("finalize: %v", ferr)
	}

	// Cross-run resume: load checkpoint and continue without replaying tools.
	state, err := session.LoadResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatalf("LoadResumeState: %v", err)
	}
	start := session.PrepareFileStart(state, fingerprint, path, session.PrepareOpts{
		CurrentModel: "fake-model",
		TemplateHash: "tmpl-hash",
	})
	if start.Mode != session.ModeContinue {
		t.Fatalf("start mode = %q, want continue", start.Mode)
	}
	if len(start.Messages) < 4 {
		t.Fatalf("expected resumed messages with tool history, got %d", len(start.Messages))
	}

	// Round 3 on resume: task_done immediately (no more tools).
	client2 := &scriptedClient{
		failAfter: -1,
		responses: []*llm.ChatResponse{taskDoneResponse()},
	}
	sh2 := session.New(repoDir, "main", "fake-model", session.SessionOptions{
		ReviewMode:  session.ReviewModeFullScan,
		ResumedFrom: sessionID,
	})
	collector2 := tool.NewCommentCollector()
	for _, cm := range start.SeedComments {
		collector2.Add(cm)
	}
	r2 := NewRunner(Deps{
		LLMClient:        client2,
		Model:            "fake-model",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: 10},
		Tools:            reg,
		CommentCollector: collector2,
		Session:          sh2,
	})
	BindSessionCheckpoint(r2, sh2, fingerprint, start.PlanGuidance, "fake-model", "tmpl-hash")
	r2.SetCompletedRounds(start.Round)

	// Reset tool counters to prove resume path does not re-call prior tools.
	find.calls.Store(0)
	read.calls.Store(0)

	completed, _, err = r2.RunPerFile(context.Background(), start.Messages, path)
	if err != nil {
		t.Fatalf("resume RunPerFile: %v", err)
	}
	if !completed {
		t.Fatal("expected task_done on resume")
	}
	if find.calls.Load() != 0 || read.calls.Load() != 0 {
		t.Fatalf("resume must not re-run tools: find=%d read=%d", find.calls.Load(), read.calls.Load())
	}
	// review_item_done with empty comments is treated as incomplete by LoadResumeState;
	// seed a finding so the done record is reusable.
	doneComments := []model.LlmComment{{Path: path, Content: "finding", StartLine: 1, EndLine: 1}}
	sh2.RecordReviewItemDone(path, path, path, fingerprint, doneComments)
	if err := sh2.Finalize(); err != nil {
		t.Fatal(err)
	}

	// Final resume state: file is done, no leftover conversation checkpoint.
	final, err := session.LoadResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := final.Item(fingerprint); !ok {
		t.Fatal("expected done item after resume success")
	}
	if _, ok := final.Conversation(fingerprint); ok {
		t.Fatal("conversation checkpoint must be cleared after done")
	}
}

func TestE2E_SameRunContinueFromCheckpoint(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := filepath.Join(t.TempDir(), "repo")
	path := "a.go"
	fp := "fp-a"

	find := &countingFileFind{}
	reg := tool.NewRegistry()
	reg.Register(find)

	// First attempt: one tool round then timeout.
	// Second attempt (same session): continue → task_done.
	client := &scriptedClient{
		failAfter: 1, // second Completions call times out on first RunPerFile
		responses: []*llm.ChatResponse{
			toolCallResp("c1", "file_find", `{"query_name":"x"}`),
			// After retry we rebind a new client below
		},
	}
	sh := session.New(repoDir, "main", "m", session.SessionOptions{ReviewMode: session.ReviewModeFullScan})
	r := NewRunner(Deps{
		LLMClient:        client,
		Model:            "m",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: 10},
		Tools:            reg,
		CommentCollector: tool.NewCommentCollector(),
		Session:          sh,
	})
	BindSessionCheckpoint(r, sh, fp, "", "m", "h")
	msgs := []llm.Message{llm.NewTextMessage("user", "go")}
	_, _, err := r.RunPerFile(context.Background(), msgs, path)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if find.calls.Load() != 1 {
		t.Fatalf("find calls = %d", find.calls.Load())
	}
	cp, ok := sh.LastConversationCheckpoint(fp)
	if !ok || cp.Round != 1 {
		t.Fatalf("checkpoint missing: ok=%v", ok)
	}

	// Same-run continue with new scripted client ending in task_done.
	client2 := &scriptedClient{failAfter: -1, responses: []*llm.ChatResponse{taskDoneResponse()}}
	r2 := NewRunner(Deps{
		LLMClient:        client2,
		Model:            "m",
		Template:         template.Template{MaxTokens: 100000, MaxToolRequestTimes: 10},
		Tools:            reg,
		CommentCollector: tool.NewCommentCollector(),
		Session:          sh,
	})
	BindSessionCheckpoint(r2, sh, fp, cp.PlanGuidance, "m", "h")
	r2.SetCompletedRounds(cp.Round)
	find.calls.Store(0)
	completed, _, err := r2.RunPerFile(context.Background(), cp.Messages, path)
	if err != nil || !completed {
		t.Fatalf("continue: completed=%v err=%v", completed, err)
	}
	if find.calls.Load() != 0 {
		t.Fatalf("same-run continue re-ran tools: %d", find.calls.Load())
	}
	_ = sh.Finalize()
}

func TestE2E_FingerprintChangeForcesCold(t *testing.T) {
	resume := &session.ResumeState{
		Conversations: map[string]session.ConversationCheckpoint{
			"fp-old": {
				Fingerprint: "fp-old",
				Messages:    []llm.Message{llm.NewTextMessage("user", "old")},
				Round:       3,
				Model:       "m",
				TemplateHash: "h",
			},
		},
	}
	// Content changed → different fingerprint → cold
	start := session.PrepareFileStart(resume, "fp-new", "a.go", session.PrepareOpts{
		CurrentModel: "m", TemplateHash: "h",
	})
	if start.Mode != session.ModeCold {
		t.Fatalf("mode = %q", start.Mode)
	}
}

func TestE2E_PartialCommentsSeedWithoutDuplicate(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := filepath.Join(t.TempDir(), "repo")
	sh := session.New(repoDir, "main", "m", session.SessionOptions{ReviewMode: session.ReviewModeFullScan})
	fp := "fp-p"
	cm := model.LlmComment{Path: "a.go", Content: "issue", StartLine: 1, EndLine: 2}
	sh.SaveConversationCheckpoint(session.ConversationCheckpoint{
		FilePath: "a.go", Fingerprint: fp, Messages: []llm.Message{llm.NewTextMessage("user", "x")},
		Round: 1, Comments: []model.LlmComment{cm}, Model: "m", TemplateHash: "h",
	})
	sh.RecordReviewItemPartial("a.go", fp, session.PhaseMain, "plan", "timeout", 1, []model.LlmComment{cm})
	sh.RecordReviewItemFailed("a.go", "a.go", "a.go", fp, "timeout", []model.LlmComment{cm})
	_ = sh.Finalize()

	state, err := session.LoadResumeState(repoDir, sh.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	start := session.PrepareFileStart(state, fp, "a.go", session.PrepareOpts{CurrentModel: "m", TemplateHash: "h"})
	if start.Mode != session.ModeContinue {
		t.Fatalf("mode=%s", start.Mode)
	}
	col := tool.NewCommentCollector()
	col.Add(cm)
	col.Add(cm) // dedupe
	if len(col.Comments()) != 1 {
		t.Fatalf("dedupe failed: %d", len(col.Comments()))
	}
	for _, c := range start.SeedComments {
		col.Add(c)
	}
	if len(col.Comments()) != 1 {
		t.Fatalf("seed re-add created duplicates: %d", len(col.Comments()))
	}
}

func TestE2E_HardKillAfterFlush_ResumeLoadsCheckpoint(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := filepath.Join(t.TempDir(), "repo")
	sh := session.New(repoDir, "main", "m", session.SessionOptions{ReviewMode: session.ReviewModeFullScan})
	fp := "fp-k"
	sh.SaveConversationCheckpoint(session.ConversationCheckpoint{
		FilePath: "k.go", Fingerprint: fp,
		Messages: []llm.Message{
			llm.NewTextMessage("user", "u"),
			llm.NewToolResultMessage("c1", "tool-out"),
		},
		Round: 1, Model: "m", TemplateHash: "h", Status: session.CheckpointInProgress,
	})
	// Simulate hard kill: no Finalize / session_end.
	sessionID := sh.SessionID
	// Drop sh without Finalize — JSONL should still have flushed checkpoint.
	path, err := session.SessionFilePath(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	// Ensure file is readable after "kill"
	time.Sleep(10 * time.Millisecond)

	state, err := session.LoadResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	cp, ok := state.Conversation(fp)
	if !ok || cp.Round != 1 {
		t.Fatalf("expected durable checkpoint, ok=%v", ok)
	}
	// Close writer so Windows can clean TempDir.
	_ = sh.Finalize()
}
