package viewer

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alibaba/open-code-review/internal/session"
)

type dataErrorReader struct {
	data []byte
	err  error
}

func (r *dataErrorReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

func TestReadJSONLLinesDiscardsPartialDataOnNonEOFError(t *testing.T) {
	errRead := errors.New("read failed")
	r := &dataErrorReader{data: []byte("complete\npartial"), err: errRead}
	var visited []string

	err := readJSONLLines(r, func(line []byte) {
		visited = append(visited, string(line))
	})
	if !errors.Is(err, errRead) {
		t.Fatalf("readJSONLLines error = %v, want %v", err, errRead)
	}
	if len(visited) != 1 || visited[0] != "complete\n" {
		t.Fatalf("visited records = %q, want only the complete line", visited)
	}
}

func TestSessionsRoot(t *testing.T) {
	root, err := SessionsRoot()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".opencodereview", "sessions")
	if root != expected {
		t.Errorf("SessionsRoot() = %q, want %q", root, expected)
	}
}

func TestLoadSession_FullParse(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "sess1.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-10T08:00:00Z","cwd":"/home/dev/proj","gitBranch":"feat","model":"claude-3","reviewMode":"commit","diffFrom":"aaa","diffTo":"bbb","diffCommit":"ccc"}`,
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":1,"messages":[{"role":"user","content":"review this"}]}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","content":"Code looks good","duration_ms":1500,"model":"claude-3","usage":{"prompt_tokens":100,"completion_tokens":50,"cache_read_tokens":10,"cache_write_tokens":5},"tool_calls":[{"name":"search","arguments":"query"}]}`,
		`{"type":"tool_call","filePath":"main.go","taskType":"main_task","result":"found 3 results","ok":true,"duration_ms":20}`,
		`{"type":"llm_request","filePath":"util.go","taskType":"plan_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"util.go","taskType":"plan_task","content":"planning","duration_ms":800,"model":"claude-3","usage":{"prompt_tokens":200,"completion_tokens":80,"cache_read_tokens":0,"cache_write_tokens":0}}`,
		`{"type":"session_end","duration_seconds":120.5,"files_reviewed":["main.go","util.go"],"llm_failures":1}`,
	)

	vs, err := LoadSession(root, "repo", "sess1")
	if err != nil {
		t.Fatal(err)
	}

	// Check summary
	if vs.Summary.SessionID != "sess1" {
		t.Errorf("SessionID = %q", vs.Summary.SessionID)
	}
	if vs.Summary.CWD != "/home/dev/proj" {
		t.Errorf("CWD = %q", vs.Summary.CWD)
	}
	if vs.Summary.GitBranch != "feat" {
		t.Errorf("GitBranch = %q", vs.Summary.GitBranch)
	}
	if vs.Summary.Model != "claude-3" {
		t.Errorf("Model = %q", vs.Summary.Model)
	}
	if vs.Summary.ReviewMode != "commit" {
		t.Errorf("ReviewMode = %q", vs.Summary.ReviewMode)
	}
	if vs.Summary.DiffFrom != "aaa" {
		t.Errorf("DiffFrom = %q", vs.Summary.DiffFrom)
	}
	if vs.Summary.DiffTo != "bbb" {
		t.Errorf("DiffTo = %q", vs.Summary.DiffTo)
	}
	if vs.Summary.DiffCommit != "ccc" {
		t.Errorf("DiffCommit = %q", vs.Summary.DiffCommit)
	}
	if vs.Summary.DurationSec != 120.5 {
		t.Errorf("DurationSec = %f", vs.Summary.DurationSec)
	}
	if vs.Summary.FileCount != 2 {
		t.Errorf("FileCount = %d", vs.Summary.FileCount)
	}
	if vs.Summary.LLMFailures != 1 {
		t.Errorf("LLMFailures = %d", vs.Summary.LLMFailures)
	}

	// Check files are sorted
	if len(vs.Files) != 2 {
		t.Fatalf("Files count = %d, want 2", len(vs.Files))
	}
	if vs.Files[0].FilePath != "main.go" {
		t.Errorf("Files[0] = %q, want main.go", vs.Files[0].FilePath)
	}
	if vs.Files[1].FilePath != "util.go" {
		t.Errorf("Files[1] = %q, want util.go", vs.Files[1].FilePath)
	}

	// Check main.go task card
	mainCards := vs.Files[0].Tasks[MainTask]
	if len(mainCards) != 1 {
		t.Fatalf("main.go main_task cards = %d", len(mainCards))
	}
	card := mainCards[0]
	if card.RequestNo != 1 {
		t.Errorf("RequestNo = %d", card.RequestNo)
	}
	if card.ResponseContent != "Code looks good" {
		t.Errorf("ResponseContent = %q", card.ResponseContent)
	}
	if card.DurationMs != 1500 {
		t.Errorf("DurationMs = %d", card.DurationMs)
	}
	if card.Model != "claude-3" {
		t.Errorf("Model = %q", card.Model)
	}
	if card.PromptTokens != 100 {
		t.Errorf("PromptTokens = %d", card.PromptTokens)
	}
	if card.CompletionTokens != 50 {
		t.Errorf("CompletionTokens = %d", card.CompletionTokens)
	}
	if card.CacheReadTokens != 10 {
		t.Errorf("CacheReadTokens = %d", card.CacheReadTokens)
	}
	if card.CacheWriteTokens != 5 {
		t.Errorf("CacheWriteTokens = %d", card.CacheWriteTokens)
	}

	// Check tool calls
	if len(card.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d", len(card.ToolCalls))
	}
	tc := card.ToolCalls[0]
	if tc.Name != "search" {
		t.Errorf("ToolCall.Name = %q", tc.Name)
	}
	if !tc.Ok {
		t.Error("ToolCall.Ok = false, want true")
	}
	if tc.Result != "found 3 results" {
		t.Errorf("ToolCall.Result = %q", tc.Result)
	}
	if tc.DurationMs != 20 {
		t.Errorf("ToolCall.DurationMs = %d", tc.DurationMs)
	}

	// Check token usage
	if vs.TokenUsage.TotalPromptTokens != 300 {
		t.Errorf("TotalPromptTokens = %d, want 300", vs.TokenUsage.TotalPromptTokens)
	}
	if vs.TokenUsage.TotalCompletionTokens != 130 {
		t.Errorf("TotalCompletionTokens = %d, want 130", vs.TokenUsage.TotalCompletionTokens)
	}
	if vs.TokenUsage.TotalCacheReadTokens != 10 {
		t.Errorf("TotalCacheReadTokens = %d", vs.TokenUsage.TotalCacheReadTokens)
	}
	if vs.TokenUsage.TotalCacheWriteTokens != 5 {
		t.Errorf("TotalCacheWriteTokens = %d", vs.TokenUsage.TotalCacheWriteTokens)
	}
	if vs.TokenUsage.RequestCount != 2 {
		t.Errorf("RequestCount = %d, want 2", vs.TokenUsage.RequestCount)
	}

	// Check file token breakdown
	if len(vs.TokenUsage.FileTokenBreakdown) != 2 {
		t.Fatalf("FileTokenBreakdown count = %d", len(vs.TokenUsage.FileTokenBreakdown))
	}
	// Sorted by total tokens (descending), util.go (200+80=280) > main.go (100+50=150)
	if vs.TokenUsage.FileTokenBreakdown[0].FilePath != "util.go" {
		t.Errorf("top token file = %q, want util.go", vs.TokenUsage.FileTokenBreakdown[0].FilePath)
	}
}

func TestLoadSession_TaskDoneStates(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "terminal-states.jsonl"),
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","tool_calls":[{"name":"task_done","arguments":"{}"}]}`,
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":2,"messages":[]}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","tool_calls":[{"name":"task_done","arguments":"{\"state\":\"DONE\"}"}]}`,
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":3,"messages":[]}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","tool_calls":[{"name":"task_done","arguments":"{\"state\":\"FAILED\"}"}]}`,
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":4,"messages":[]}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","tool_calls":[{"name":"task_done","arguments":"{\"state\":\"\"}"}]}`,
		`{"type":"llm_request","filePath":"main.go","taskType":"main_task","request_no":5,"messages":[]}`,
		`{"type":"llm_response","filePath":"main.go","taskType":"main_task","tool_calls":[{"name":"task_done","arguments":"{\"state\":\"\"}"},{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}]}`,
		`{"type":"tool_call","filePath":"main.go","taskType":"main_task","tool_name":"file_read","result":"package main","ok":true,"duration_ms":20}`,
	)

	vs, err := LoadSession(root, "repo", "terminal-states")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs.Files) != 1 {
		t.Fatalf("files = %d, want 1", len(vs.Files))
	}
	cards := vs.Files[0].Tasks[MainTask]
	if len(cards) != 5 {
		t.Fatalf("main_task cards = %d, want 5", len(cards))
	}
	wantOK := []bool{true, true, false, false}
	for i, want := range wantOK {
		if len(cards[i].ToolCalls) != 1 {
			t.Fatalf("card %d tool calls = %d, want 1", i, len(cards[i].ToolCalls))
		}
		if got := cards[i].ToolCalls[0].Ok; got != want {
			t.Errorf("card %d task_done Ok = %v, want %v", i, got, want)
		}
	}
	if len(cards[4].ToolCalls) != 2 {
		t.Fatalf("card 4 tool calls = %d, want 2", len(cards[4].ToolCalls))
	}
	if cards[4].ToolCalls[0].Ok || cards[4].ToolCalls[0].Result != "" {
		t.Errorf("invalid task_done received another tool's result: %+v", cards[4].ToolCalls[0])
	}
	if !cards[4].ToolCalls[1].Ok || cards[4].ToolCalls[1].Result != "package main" {
		t.Errorf("file_read result was not matched by name: %+v", cards[4].ToolCalls[1])
	}
}

func TestLoadSession_MissingFile(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	_, err := LoadSession(root, "repo", "nonexistent")
	if err == nil {
		t.Error("expected error for missing session file")
	}
}

func TestLoadSessionReadsV1Manifest(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "manifest.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"session_end","duration_seconds":1,"run_manifest":{"schema_version":"ocr.run-manifest/v1","run_id":"run-1","operation":"review","terminal_state":"complete","repository":{},"input":{"mode":"workspace"},"execution":{},"coverage":{"selected":[{"item_id":"a","path":"a.go"},{"item_id":"b","path":"b.go"}],"completed":[{"item_id":"a","path":"a.go"}],"reused":[{"item_id":"b","path":"b.go"}],"failed":[],"waived":[]},"elapsed_ms":1000}}`)

	vs, err := LoadSession(root, "repo", "manifest")
	if err != nil {
		t.Fatal(err)
	}
	if vs.Summary.RunManifest == nil || vs.Summary.TerminalState != "complete" || vs.Summary.FileCount != 2 {
		t.Fatalf("summary = %+v", vs.Summary)
	}
	if vs.Summary.CompletedCount != 1 || vs.Summary.ReusedCount != 1 || vs.Summary.FailedCount != 0 || vs.Summary.WaivedCount != 0 {
		t.Fatalf("coverage counts = %+v", vs.Summary)
	}
}

func TestViewerReadsSessionEndLargerThanScannerLimit(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	const itemCount = 35000
	items := make([]session.CoverageItem, 0, itemCount)
	for i := range itemCount {
		id := fmt.Sprintf("%064x", i)
		items = append(items, session.CoverageItem{
			ItemID:      id,
			Path:        fmt.Sprintf("pkg/file-%05d.go", i),
			Fingerprint: id,
		})
	}
	manifest := session.RunManifest{
		SchemaVersion: session.ManifestSchemaVersion,
		RunID:         "large",
		Operation:     session.OperationReview,
		TerminalState: session.StateComplete,
		Input:         session.ManifestInput{Mode: session.InputModeWorkspace},
		Coverage: session.Coverage{
			Selected:  items,
			Completed: items,
			Reused:    []session.CoverageItem{},
			Failed:    []session.CoverageItem{},
			Waived:    []session.CoverageItem{},
		},
		ElapsedMS: 1000,
	}
	sessionEndData, err := json.Marshal(map[string]any{
		"type":             "session_end",
		"duration_seconds": 1,
		"run_manifest":     manifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessionEndData) <= 10*1024*1024 {
		t.Fatalf("session_end size = %d, want more than former 10 MiB limit", len(sessionEndData))
	}
	path := filepath.Join(repoDir, "large.jsonl")
	writeJSONL(t, path,
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		string(sessionEndData),
	)

	summary, err := peekSession(path)
	if err != nil {
		t.Fatalf("peek large session: %v", err)
	}
	if summary.RunManifest == nil || summary.TerminalState != "complete" || summary.FileCount != itemCount {
		t.Fatalf("peek summary = %+v", summary)
	}

	vs, err := LoadSession(root, "repo", "large")
	if err != nil {
		t.Fatalf("load large session: %v", err)
	}
	if vs.Summary.RunManifest == nil || vs.Summary.TerminalState != "complete" || vs.Summary.FileCount != itemCount {
		t.Fatalf("loaded summary = %+v", vs.Summary)
	}
}

func TestLoadSession_MalformedLines(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "bad.jsonl"),
		`not json at all`,
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{broken json`,
		`{"type":"session_end","duration_seconds":10,"files_reviewed":[],"llm_failures":0}`,
	)

	vs, err := LoadSession(root, "repo", "bad")
	if err != nil {
		t.Fatal(err)
	}
	if vs.Summary.CWD != "/x" {
		t.Errorf("CWD = %q, want /x (should skip malformed lines)", vs.Summary.CWD)
	}
}

func TestLoadSession_LLMError(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "errs.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_error","filePath":"a.go","taskType":"main_task","error":"rate limit exceeded","duration_ms":500}`,
		`{"type":"session_end","duration_seconds":5,"files_reviewed":["a.go"],"llm_failures":1}`,
	)

	vs, err := LoadSession(root, "repo", "errs")
	if err != nil {
		t.Fatal(err)
	}

	cards := vs.Files[0].Tasks[MainTask]
	if len(cards) != 1 {
		t.Fatalf("cards count = %d", len(cards))
	}
	if cards[0].Error != "rate limit exceeded" {
		t.Errorf("Error = %q", cards[0].Error)
	}
	if cards[0].DurationMs != 500 {
		t.Errorf("DurationMs = %d", cards[0].DurationMs)
	}
}

func TestLoadSession_ToolCallWithOkFalse(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "tc.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"main_task","content":"result","duration_ms":100,"model":"m","usage":{"prompt_tokens":10,"completion_tokens":5},"tool_calls":[{"name":"search","arguments":"query"}]}`,
		`{"type":"tool_call","filePath":"a.go","taskType":"main_task","result":"error: not found","ok":false,"duration_ms":50}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":["a.go"]}`,
	)

	vs, err := LoadSession(root, "repo", "tc")
	if err != nil {
		t.Fatal(err)
	}

	cards := vs.Files[0].Tasks[MainTask]
	if len(cards) != 1 {
		t.Fatalf("cards = %d", len(cards))
	}
	if len(cards[0].ToolCalls) != 1 {
		t.Fatalf("tool calls = %d", len(cards[0].ToolCalls))
	}
	tc := cards[0].ToolCalls[0]
	if tc.Ok {
		t.Error("expected Ok=false")
	}
	if tc.Result != "error: not found" {
		t.Errorf("Result = %q", tc.Result)
	}
	if tc.DurationMs != 50 {
		t.Errorf("DurationMs = %d", tc.DurationMs)
	}
}

func TestLoadSession_MultipleRequests(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "multi.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"main_task","content":"first pass","duration_ms":100,"model":"m","usage":{"prompt_tokens":50,"completion_tokens":20}}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":2,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"main_task","content":"second pass","duration_ms":200,"model":"m","usage":{"prompt_tokens":60,"completion_tokens":30}}`,
		`{"type":"session_end","duration_seconds":10,"files_reviewed":["a.go"]}`,
	)

	vs, err := LoadSession(root, "repo", "multi")
	if err != nil {
		t.Fatal(err)
	}

	cards := vs.Files[0].Tasks[MainTask]
	if len(cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(cards))
	}
	if cards[0].ResponseContent != "first pass" {
		t.Errorf("cards[0] content = %q", cards[0].ResponseContent)
	}
	if cards[1].ResponseContent != "second pass" {
		t.Errorf("cards[1] content = %q", cards[1].ResponseContent)
	}
	if vs.TokenUsage.RequestCount != 2 {
		t.Errorf("RequestCount = %d", vs.TokenUsage.RequestCount)
	}
}

func TestLoadSession_EmptyFile(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(repoDir, "empty.jsonl"))

	vs, err := LoadSession(root, "repo", "empty")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs.Files) != 0 {
		t.Errorf("Files = %d, want 0", len(vs.Files))
	}
}

func TestLoadSession_ResponseWithoutRequest(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// llm_response for a file that has no prior request - should not panic
	writeJSONL(t, filepath.Join(repoDir, "orphan.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_response","filePath":"unknown.go","taskType":"main_task","content":"orphan","duration_ms":100,"model":"m"}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":[]}`,
	)

	vs, err := LoadSession(root, "repo", "orphan")
	if err != nil {
		t.Fatal(err)
	}
	// No file groups should be created for orphan responses (no request created the fileIndex entry)
	if len(vs.Files) != 0 {
		t.Errorf("Files = %d, want 0 (orphan response has no request)", len(vs.Files))
	}
}

func TestLoadSession_LLMErrorWithoutRequest(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "orphanerr.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_error","filePath":"unknown.go","taskType":"main_task","error":"fail","duration_ms":10}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":[]}`,
	)

	// Should not panic
	vs, err := LoadSession(root, "repo", "orphanerr")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs.Files) != 0 {
		t.Errorf("Files = %d", len(vs.Files))
	}
}

func TestLoadSession_ToolCallWithoutRequest(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "orphantc.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"tool_call","filePath":"unknown.go","taskType":"main_task","result":"x","ok":true}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":[]}`,
	)

	// Should not panic
	vs, err := LoadSession(root, "repo", "orphantc")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs.Files) != 0 {
		t.Errorf("Files = %d", len(vs.Files))
	}
}

func TestDiscoverRepos_SkipsUnreadableSubdir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission checks are bypassed for root")
	}
	root := t.TempDir()
	badRepo := filepath.Join(root, "unreadable-repo")
	if err := os.MkdirAll(badRepo, 0755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(badRepo, "s.jsonl"), `{"type":"session_start"}`)
	// Remove read permission so os.ReadDir(repoDir) fails
	if err := os.Chmod(badRepo, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(badRepo, 0755) })

	repos, err := DiscoverRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	// Unreadable repo is skipped (continue)
	if len(repos) != 0 {
		t.Errorf("repos = %d, want 0 (unreadable dir skipped)", len(repos))
	}
}

func TestListSessions_SkipsUnreadableFiles(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission checks are bypassed for root")
	}
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a valid session file
	writeJSONL(t, filepath.Join(repoDir, "good.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`)

	// Create an unreadable jsonl file
	badPath := filepath.Join(repoDir, "bad.jsonl")
	if err := os.WriteFile(badPath, []byte(`{"type":"session_start"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badPath, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(badPath, 0644) })

	sessions, err := ListSessions(root, "repo")
	if err != nil {
		t.Fatal(err)
	}
	// Should get 1 session (the good one), bad one is skipped
	if len(sessions) != 1 {
		t.Errorf("sessions = %d, want 1 (bad file skipped)", len(sessions))
	}
}

func TestLoadSession_MultipleTaskTypes(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "tasks.jsonl"),
		`{"type":"session_start","timestamp":"2025-01-01T00:00:00Z","cwd":"/x","model":"m"}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"plan_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"plan_task","content":"planning","duration_ms":100,"model":"m","usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"main_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"main_task","content":"reviewing","duration_ms":200,"model":"m","usage":{"prompt_tokens":20,"completion_tokens":10}}`,
		`{"type":"llm_request","filePath":"a.go","taskType":"memory_compression_task","request_no":1,"messages":[]}`,
		`{"type":"llm_response","filePath":"a.go","taskType":"memory_compression_task","content":"compressed","duration_ms":50,"model":"m","usage":{"prompt_tokens":5,"completion_tokens":3}}`,
		`{"type":"session_end","duration_seconds":5,"files_reviewed":["a.go"]}`,
	)

	vs, err := LoadSession(root, "repo", "tasks")
	if err != nil {
		t.Fatal(err)
	}

	if len(vs.Files) != 1 {
		t.Fatalf("Files = %d", len(vs.Files))
	}
	fg := vs.Files[0]
	if len(fg.Tasks) != 3 {
		t.Errorf("task types = %d, want 3", len(fg.Tasks))
	}
	if len(fg.Tasks[PlanTask]) != 1 {
		t.Errorf("plan_task cards = %d", len(fg.Tasks[PlanTask]))
	}
	if len(fg.Tasks[MainTask]) != 1 {
		t.Errorf("main_task cards = %d", len(fg.Tasks[MainTask]))
	}
	if len(fg.Tasks[MemoryCompressionTask]) != 1 {
		t.Errorf("memory_compression_task cards = %d", len(fg.Tasks[MemoryCompressionTask]))
	}
}

func TestLoadSession_ReviewItemDone(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "sess1.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-10T08:00:00Z","cwd":"/home/dev/proj","gitBranch":"feat","model":"claude-3","reviewMode":"commit"}`,
		`{"type":"review_item_done","filePath":"src/api/users.go","comments":[{"path":"src/api/users.go","content":"SQL injection risk","suggestion_code":"const u = await db.user.findUnique({where:{id:sanitize(input)}})","existing_code":"const u = await db.$queryRawUnsafe('SELECT * FROM users WHERE id='+input)","start_line":42,"end_line":42,"category":"security","severity":"critical"}]}`,
		`{"type":"review_item_done","filePath":"src/ui/List.tsx","comments":[{"path":"src/ui/List.tsx","content":"Missing key prop","suggestion_code":"items.map(i=><li key={i.id}>{i.name}</li>)","existing_code":"items.map(i=><li>{i.name}</li>)","start_line":15,"end_line":17,"category":"bug","severity":"high"},{"path":"src/ui/List.tsx","content":"Use useMemo here","start_line":30,"end_line":30,"category":"performance","severity":"medium"}]}`,
		`{"type":"review_item_done","filePath":"src/lib/old.ts","comments":[{"path":"src/lib/old.ts","content":"Unused import","start_line":1,"end_line":1,"category":"style","severity":"low"}]}`,
		`{"type":"session_end","duration_seconds":30,"files_reviewed":["src/api/users.go","src/ui/List.tsx","src/lib/old.ts"],"llm_failures":0}`,
	)

	vs, err := LoadSession(root, "repo", "sess1")
	if err != nil {
		t.Fatal(err)
	}

	if len(vs.Findings) != 4 {
		t.Fatalf("Findings count = %d, want 4", len(vs.Findings))
	}

	// Finding 1: critical security
	f := vs.Findings[0]
	if f.FilePath != "src/api/users.go" {
		t.Errorf("Findings[0].FilePath = %q", f.FilePath)
	}
	if f.Severity != "critical" {
		t.Errorf("Findings[0].Severity = %q", f.Severity)
	}
	if f.Category != "security" {
		t.Errorf("Findings[0].Category = %q", f.Category)
	}
	if f.Content != "SQL injection risk" {
		t.Errorf("Findings[0].Content = %q", f.Content)
	}
	if f.SuggestionCode == "" {
		t.Error("Findings[0].SuggestionCode should not be empty")
	}
	if f.ExistingCode == "" {
		t.Error("Findings[0].ExistingCode should not be empty")
	}
	if f.StartLine != 42 || f.EndLine != 42 {
		t.Errorf("Findings[0] StartLine/EndLine = %d/%d", f.StartLine, f.EndLine)
	}

	// Finding 2: high bug
	f = vs.Findings[1]
	if f.Severity != "high" || f.Category != "bug" {
		t.Errorf("Findings[1] = %s/%s, want high/bug", f.Severity, f.Category)
	}

	// Finding 3: medium performance (no code diff)
	f = vs.Findings[2]
	if f.Severity != "medium" || f.Category != "performance" {
		t.Errorf("Findings[2] = %s/%s, want medium/performance", f.Severity, f.Category)
	}
	if f.SuggestionCode != "" || f.ExistingCode != "" {
		t.Error("Findings[2] should have no code diff")
	}

	// Finding 4: low style
	f = vs.Findings[3]
	if f.Severity != "low" || f.Category != "style" {
		t.Errorf("Findings[3] = %s/%s, want low/style", f.Severity, f.Category)
	}

	// Severity and category counts
	if vs.SeverityCount["critical"] != 1 {
		t.Errorf("SeverityCount[critical] = %d", vs.SeverityCount["critical"])
	}
	if vs.SeverityCount["high"] != 1 {
		t.Errorf("SeverityCount[high] = %d", vs.SeverityCount["high"])
	}
	if vs.SeverityCount["medium"] != 1 {
		t.Errorf("SeverityCount[medium] = %d", vs.SeverityCount["medium"])
	}
	if vs.SeverityCount["low"] != 1 {
		t.Errorf("SeverityCount[low] = %d", vs.SeverityCount["low"])
	}
	if vs.CategoryCount["security"] != 1 {
		t.Errorf("CategoryCount[security] = %d", vs.CategoryCount["security"])
	}
	if vs.CategoryCount["bug"] != 1 {
		t.Errorf("CategoryCount[bug] = %d", vs.CategoryCount["bug"])
	}
	if vs.CategoryCount["performance"] != 1 {
		t.Errorf("CategoryCount[performance] = %d", vs.CategoryCount["performance"])
	}
	if vs.CategoryCount["style"] != 1 {
		t.Errorf("CategoryCount[style] = %d", vs.CategoryCount["style"])
	}
}

func TestLoadSession_ReviewItemReused(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "sess1.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-10T08:00:00Z","cwd":"/home/dev/proj","gitBranch":"feat","model":"claude-3"}`,
		`{"type":"review_item_reused","filePath":"src/old.go","sourceSessionId":"prev-sess","comments":[{"path":"src/old.go","content":"Reused finding","category":"bug","severity":"high"}]}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":["src/old.go"],"llm_failures":0}`,
	)

	vs, err := LoadSession(root, "repo", "sess1")
	if err != nil {
		t.Fatal(err)
	}

	if len(vs.Findings) != 1 {
		t.Fatalf("Findings count = %d, want 1", len(vs.Findings))
	}
	if vs.Findings[0].Content != "Reused finding" {
		t.Errorf("Content = %q", vs.Findings[0].Content)
	}
}

func TestLoadSession_NoFindings(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "sess1.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-10T08:00:00Z","cwd":"/home/dev/proj","gitBranch":"feat","model":"claude-3"}`,
		`{"type":"session_end","duration_seconds":1,"files_reviewed":[],"llm_failures":0}`,
	)

	vs, err := LoadSession(root, "repo", "sess1")
	if err != nil {
		t.Fatal(err)
	}

	if len(vs.Findings) != 0 {
		t.Errorf("Findings count = %d, want 0", len(vs.Findings))
	}
	if len(vs.SeverityCount) != 0 {
		t.Errorf("SeverityCount should be empty")
	}
	if len(vs.CategoryCount) != 0 {
		t.Errorf("CategoryCount should be empty")
	}
}

func TestLoadSession_ReviewItemFailed_WithComments(t *testing.T) {
	root := t.TempDir()
	repoDir := filepath.Join(root, "repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, filepath.Join(repoDir, "sess1.jsonl"),
		`{"type":"session_start","timestamp":"2025-06-10T08:00:00Z","cwd":"/home/dev/proj","gitBranch":"feat","model":"claude-3","reviewMode":"commit"}`,
		`{"type":"review_item_failed","filePath":"src/bad.go","error":"400 Bad Request","comments":[{"path":"src/bad.go","content":"N+1 query detected","suggestion_code":"txn.find({include:{posts:true}})","existing_code":"for(user of users){await db.post.find({userId:user.id})}","start_line":10,"end_line":14,"category":"performance","severity":"high"}]}`,
		`{"type":"review_item_failed","filePath":"src/also_bad.go","error":"timeout","comments":[{"path":"src/also_bad.go","content":"Missing error handling","start_line":5,"end_line":5,"category":"bug","severity":"medium"},{"path":"src/also_bad.go","content":"Hardcoded secret","start_line":20,"end_line":20,"category":"security","severity":"critical"}]}`,
		`{"type":"session_end","duration_seconds":30,"files_reviewed":["src/bad.go","src/also_bad.go"],"llm_failures":2}`,
	)

	vs, err := LoadSession(root, "repo", "sess1")
	if err != nil {
		t.Fatal(err)
	}

	if len(vs.Findings) != 3 {
		t.Fatalf("Findings count = %d, want 3 (comments from failed files)", len(vs.Findings))
	}

	// Finding 1: N+1 query from bad.go
	f := vs.Findings[0]
	if f.FilePath != "src/bad.go" || f.Severity != "high" || f.Category != "performance" {
		t.Errorf("Findings[0] = %s/%s/%s, want bad.go/high/performance", f.FilePath, f.Severity, f.Category)
	}

	// Finding 2: missing error handling from also_bad.go
	f = vs.Findings[1]
	if f.Content != "Missing error handling" {
		t.Errorf("Findings[1].Content = %q", f.Content)
	}

	// Finding 3: hardcoded secret from also_bad.go
	f = vs.Findings[2]
	if f.Content != "Hardcoded secret" || f.Severity != "critical" {
		t.Errorf("Findings[2] = %s/%s, want Hardcoded secret/critical", f.Content, f.Severity)
	}

	// Severity counts should include failed findings
	if vs.SeverityCount["high"] != 1 || vs.SeverityCount["medium"] != 1 || vs.SeverityCount["critical"] != 1 {
		t.Errorf("SeverityCount h=%d m=%d c=%d", vs.SeverityCount["high"], vs.SeverityCount["medium"], vs.SeverityCount["critical"])
	}
	if vs.CategoryCount["performance"] != 1 || vs.CategoryCount["bug"] != 1 || vs.CategoryCount["security"] != 1 {
		t.Errorf("CategoryCount perf=%d bug=%d sec=%d", vs.CategoryCount["performance"], vs.CategoryCount["bug"], vs.CategoryCount["security"])
	}
}
