package refactor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alibaba/open-code-review/internal/config/template"
	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/scan"
	"github.com/alibaba/open-code-review/internal/session"
	"github.com/alibaba/open-code-review/internal/tool"
)

func init() { session.UseTestSessions() }

type e2eClient struct {
	responses []*llm.ChatResponse
	calls     atomic.Int64
	failAt    int
}

func (c *e2eClient) CompletionsWithCtx(ctx context.Context, _ llm.ChatRequest) (*llm.ChatResponse, error) {
	n := int(c.calls.Add(1) - 1)
	if c.failAt >= 0 && n >= c.failAt {
		return nil, context.DeadlineExceeded
	}
	if n >= len(c.responses) {
		return nil, errors.New("no response")
	}
	return c.responses[n], nil
}

type e2eFind struct{ n atomic.Int64 }

func (f *e2eFind) Tool() tool.Tool { return tool.FileFind }
func (f *e2eFind) Execute(context.Context, map[string]any) (string, error) {
	f.n.Add(1)
	return "log-helper.ts", nil
}

func respTool(id, name, args string) *llm.ChatResponse {
	empty := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			Content: &empty,
			ToolCalls: []llm.ToolCall{{
				ID: id, Type: "function",
				Function: llm.FunctionCall{Name: name, Arguments: args},
			}},
		}}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 2},
	}
}

func respDone() *llm.ChatResponse {
	empty := ""
	return &llm.ChatResponse{
		Choices: []llm.Choice{{Message: llm.ResponseMessage{
			Content: &empty,
			ToolCalls: []llm.ToolCall{{
				ID: "done", Type: "function",
				Function: llm.FunctionCall{Name: "task_done", Arguments: `{"state":"DONE"}`},
			}},
		}}},
		Model: "fake",
		Usage: &llm.UsageInfo{PromptTokens: 5, CompletionTokens: 2},
	}
}

// TestE2E_RefactorAgent_MidFileResume exercises the full agent path:
// timeout after tools → JSONL checkpoint → --resume continue without re-running tools.
func TestE2E_RefactorAgent_MidFileResume(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := t.TempDir()
	src := filepath.Join(repoDir, "operation_queue.go")
	if err := os.WriteFile(src, []byte("package main\nfunc Op() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	find := &e2eFind{}
	reg := tool.NewRegistry()
	reg.Register(find)

	tpl := template.RefactorTemplate{
		MainTask: template.LlmConversation{
			Messages: []template.ChatMessage{
				{Role: "system", Content: "analyze {{current_file_path}} {{plan_guidance}}"},
				{Role: "user", Content: "file content follows"},
			},
		},
		MaxTokens:           100000,
		MaxToolRequestTimes: 10,
		MaxFileSizeBytes:    1 << 20,
	}

	// Run 1: successful tool round then timeout. maxRetries=3 but after first
	// successful checkpoint, subsequent attempts continue mid-file and still
	// timeout (client fails every Completions after the first).
	client1 := &e2eClient{
		failAt: 1,
		responses: []*llm.ChatResponse{
			respTool("c1", "file_find", `{"query_name":"log-helper"}`),
		},
	}
	sh1 := session.New(repoDir, "main", "fake", session.SessionOptions{ReviewMode: session.ReviewModeFullScan})
	a1 := NewAgent(Args{
		RepoDir:               repoDir,
		Paths:                 []string{"operation_queue.go"},
		Template:              tpl,
		LLMClient:             client1,
		Tools:                 reg,
		CommentCollector:      tool.NewCommentCollector(),
		MaxConcurrency:        1,
		ConcurrentTaskTimeout: 0,
		Model:                 "fake",
		Session:               sh1,
		SkipPlan:              true,
	})

	_, _ = a1.Run(context.Background())
	sessionID := sh1.SessionID

	state, err := session.LoadResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatalf("LoadResumeState: %v", err)
	}
	fp := scan.ScanItemFingerprint("operation_queue.go")
	cp, ok := state.Conversation(fp)
	if !ok {
		// List keys for debug
		keys := make([]string, 0, len(state.Conversations))
		for k := range state.Conversations {
			keys = append(keys, k)
		}
		t.Fatalf("expected conversation checkpoint for %s; have %v failed=%v", fp, keys, state.FailedFiles)
	}
	if cp.Round < 1 {
		t.Fatalf("checkpoint round = %d, want >= 1", cp.Round)
	}
	if find.n.Load() < 1 {
		t.Fatal("file_find should have run in run1")
	}

	// Run 2: resume mid-file → task_done only.
	find.n.Store(0)
	client2 := &e2eClient{failAt: -1, responses: []*llm.ChatResponse{respDone()}}
	sh2 := session.New(repoDir, "main", "fake", session.SessionOptions{
		ReviewMode:  session.ReviewModeFullScan,
		ResumedFrom: sessionID,
	})
	// Seed a finding so review_item_done is treated as complete (empty comments
	// are treated as incomplete by LoadResumeState).
	collector2 := tool.NewCommentCollector()
	collector2.Add(model.LlmComment{Path: "operation_queue.go", Content: "refactor note", StartLine: 1, EndLine: 1})
	a2 := NewAgent(Args{
		RepoDir:          repoDir,
		Paths:            []string{"operation_queue.go"},
		Template:         tpl,
		LLMClient:        client2,
		Tools:            reg,
		CommentCollector: collector2,
		MaxConcurrency:   1,
		Model:            "fake",
		Session:          sh2,
		SkipPlan:         true,
		Resume:           state,
		ResumeMode:       session.ResumeModeContinue,
	})

	comments, err := a2.Run(context.Background())
	if err != nil {
		// "all N failed" should not happen if continue + task_done worked.
		if strings.Contains(err.Error(), "all") {
			t.Fatalf("resume failed entirely: %v", err)
		}
		t.Fatalf("resume run: %v", err)
	}
	_ = comments
	if find.n.Load() != 0 {
		t.Fatalf("mid-file resume re-ran file_find %d time(s); expected 0", find.n.Load())
	}

	final, err := session.LoadResumeState(repoDir, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := final.Item(fp); !ok {
		t.Fatal("expected review_item_done after successful resume")
	}
	if _, ok := final.Conversation(fp); ok {
		t.Fatal("checkpoint should be cleared after done")
	}
}

func TestE2E_RefactorAgent_RestartFailedIsCold(t *testing.T) {
	resume := &session.ResumeState{
		SessionID: "s",
		Conversations: map[string]session.ConversationCheckpoint{
			"fp": {
				Fingerprint:  "fp",
				Messages:     []llm.Message{llm.NewTextMessage("user", "x")},
				Round:        2,
				Model:        "fake",
				TemplateHash: "h",
			},
		},
	}
	start := session.PrepareFileStart(resume, "fp", "a.go", session.PrepareOpts{
		ResumeMode:   session.ResumeModeRestartFailed,
		CurrentModel: "fake",
		TemplateHash: "h",
	})
	if start.Mode != session.ModeCold {
		t.Fatalf("restart-failed mode = %q", start.Mode)
	}
}

// TestApplyResume_SkipsFilesNotInPriorSession ensures --resume does not
// re-analyze the whole project when only one file failed in the prior session.
func TestApplyResume_SkipsFilesNotInPriorSession(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	failedPath := "operation_queue.go"
	fp := scan.ScanItemFingerprint(failedPath)
	resume := &session.ResumeState{
		SessionID:   "sess-1",
		FailedFiles: map[string]string{fp: failedPath},
		Items:       map[string]session.ResumeItem{},
	}

	// Agent with resume: enumerate would see many files; applyResume filters.
	a := NewAgent(Args{
		RepoDir:          t.TempDir(),
		Model:            "fake",
		Resume:           resume,
		ResumeMode:       session.ResumeModeRestartFailed,
		CommentCollector: tool.NewCommentCollector(),
		Tools:            tool.NewRegistry(),
		SkipPlan:         true,
	})

	items := []model.ScanItem{
		{Path: failedPath, Content: "package main\n"},
		{Path: "unrelated/a.go", Content: "package a\n"},
		{Path: "unrelated/b.go", Content: "package b\n"},
		{Path: "unrelated/c.go", Content: "package c\n"},
	}
	got := a.applyResume(items)
	if len(got) != 1 {
		t.Fatalf("dispatch count = %d, want 1 (only failed session file); got %+v", len(got), got)
	}
	if got[0].Path != failedPath {
		t.Fatalf("dispatched %q, want %q", got[0].Path, failedPath)
	}
	_ = a.Session().Finalize()
}

func TestE2E_RefactorLocalApply(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := t.TempDir()
	src := "package main\n\nfunc legacyName() {\n\tprintln(\"old\")\n}\n"
	p := filepath.Join(repoDir, "app.go")
	if err := os.WriteFile(p, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}

	reg := tool.NewRegistry()
	tpl := template.RefactorTemplate{
		MainTask: template.LlmConversation{
			Messages: []template.ChatMessage{
				{Role: "system", Content: "analyze {{current_file_path}}"},
				{Role: "user", Content: "file content follows"},
			},
		},
		MaxTokens:         100000,
		MaxToolRequestTimes: 10,
		MaxFileSizeBytes:  1 << 20,
	}

	client := &e2eClient{failAt: -1, responses: []*llm.ChatResponse{respDone()}}
	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{
		Path:           "app.go",
		StartLine:      3,
		EndLine:        3,
		Content:        "Rename legacyName to newName",
		SuggestionCode: "func newName() {",
		Category:       "maintainability",
		Severity:       "medium",
	})

	sh := session.New(repoDir, "main", "fake", session.SessionOptions{ReviewMode: session.ReviewModeFullScan})
	ag := NewAgent(Args{
		RepoDir:          repoDir,
		Paths:            []string{"app.go"},
		Template:         tpl,
		LLMClient:        client,
		Tools:            reg,
		CommentCollector: collector,
		MaxConcurrency:   1,
		Model:            "fake",
		Session:          sh,
		SkipPlan:         true,
		Mode:             "local",
		Apply:            true,
		ApplyRunTests:    false,
	})

	_, err := ag.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "func newName()") {
		t.Fatalf("file was not applied; content: %s", string(data))
	}
	if strings.Contains(string(data), "func legacyName()") {
		t.Fatalf("old function name still present: %s", string(data))
	}
}

func TestE2E_RefactorLocalApply_Rollback(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	repoDir := t.TempDir()
	src := "package main\n\nfunc foo() {\n\tprintln(\"ok\")\n}\n"
	p := filepath.Join(repoDir, "app.go")
	os.WriteFile(p, []byte(src), 0o600)

	reg := tool.NewRegistry()
	tpl := template.RefactorTemplate{
		MainTask: template.LlmConversation{
			Messages: []template.ChatMessage{
				{Role: "system", Content: "analyze {{current_file_path}}"},
			},
		},
		MaxTokens: 100000, MaxToolRequestTimes: 10, MaxFileSizeBytes: 1 << 20,
	}
	client := &e2eClient{failAt: -1, responses: []*llm.ChatResponse{respDone()}}

	collector := tool.NewCommentCollector()
	collector.Add(model.LlmComment{
		Path:           "app.go",
		StartLine:      3,
		EndLine:        3,
		Content:        "bad change",
		SuggestionCode: "func foo( {", // syntax error
	})

	sh := session.New(repoDir, "main", "fake", session.SessionOptions{ReviewMode: session.ReviewModeFullScan})
	ag := NewAgent(Args{
		RepoDir:          repoDir,
		Paths:            []string{"app.go"},
		Template:         tpl,
		LLMClient:        client,
		Tools:            reg,
		CommentCollector: collector,
		MaxConcurrency:   1,
		Model:            "fake",
		Session:          sh,
		SkipPlan:         true,
		Mode:             "local",
		Apply:            true,
	})

	_, err := ag.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed (apply failures should not fail the run): %v", err)
	}

	data, _ := os.ReadFile(p)
	if string(data) != src {
		t.Fatalf("file was not rolled back; got: %s", string(data))
	}
}
