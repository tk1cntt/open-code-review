package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
)

// ResumeState is the replayed, read-only checkpoint index for one prior session.
type ResumeState struct {
	SessionID  string
	RepoDir    string
	GitBranch  string
	Model      string
	ReviewMode string
	DiffFrom   string
	DiffTo     string
	DiffCommit string
	Items      map[string]ResumeItem
	// FailedFiles maps fingerprint → newPath for files that failed in the
	// previous session, so callers can log which files are being retried.
	FailedFiles map[string]string
	// Conversations maps fingerprint → latest mid-file conversation checkpoint.
	Conversations map[string]ConversationCheckpoint
	// Partials maps fingerprint → latest review_item_partial record.
	Partials map[string]PartialItem
	// CorruptLines is the number of JSONL lines skipped during load.
	CorruptLines int
}

// ResumeItem is a completed file-level checkpoint, keyed by diff fingerprint.
type ResumeItem struct {
	FilePath    string
	OldPath     string
	NewPath     string
	Fingerprint string
	Comments    []model.LlmComment
}

// PartialItem is a lightweight partial-findings record for a failed/interrupted file.
type PartialItem struct {
	FilePath     string
	Fingerprint  string
	Phase        string
	PlanGuidance string
	Round        int
	StopReason   string
	Comments     []model.LlmComment
}

type resumeRecord struct {
	Type                string             `json:"type"`
	SessionID           string             `json:"sessionId"`
	Cwd                 string             `json:"cwd"`
	GitBranch           string             `json:"gitBranch"`
	Model               string             `json:"model"`
	ReviewMode          string             `json:"reviewMode"`
	DiffFrom            string             `json:"diffFrom"`
	DiffTo              string             `json:"diffTo"`
	DiffCommit          string             `json:"diffCommit"`
	FilePath            string             `json:"filePath"`
	OldPath             string             `json:"oldPath"`
	NewPath             string             `json:"newPath"`
	Fingerprint         string             `json:"fingerprint"`
	SourceSessionID     string             `json:"sourceSessionId"`
	Error               string             `json:"error"`
	Comments            []model.LlmComment `json:"comments"`
	Phase               string             `json:"phase"`
	PlanGuidance        string             `json:"planGuidance"`
	Round               int                `json:"round"`
	TemplateHash        string             `json:"templateHash"`
	Messages            []llm.Message      `json:"messages"`
	CommentFingerprints []string           `json:"commentFingerprints"`
	Status              string             `json:"status"`
	StopReason          string             `json:"stopReason"`
}

// SessionFilePath returns the JSONL path for a persisted session.
func SessionFilePath(repoDir, sessionID string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("session id is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".opencodereview", sessionSubDir, encodeRepoPath(repoDir), sessionID+".jsonl"), nil
}

// LoadResumeState replays a previous session JSONL into a fingerprint index.
// Corrupt / truncated JSONL lines are skipped with a warning so a hard crash
// mid-write cannot block the entire resume.
func LoadResumeState(repoDir, sessionID string) (*ResumeState, error) {
	path, err := SessionFilePath(repoDir, sessionID)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open resume session %q: %w", sessionID, err)
	}
	defer f.Close()

	state := &ResumeState{
		SessionID:     sessionID,
		RepoDir:       repoDir,
		Items:         make(map[string]ResumeItem),
		FailedFiles:   make(map[string]string),
		Conversations: make(map[string]ConversationCheckpoint),
		Partials:      make(map[string]PartialItem),
	}
	reader := bufio.NewReader(f)
	lineNo := 0
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNo++
			// Drop trailing newline for cleaner errors; Unmarshal tolerates whitespace.
			trimmed := bytesTrimSpace(line)
			if len(trimmed) == 0 {
				// skip blank
			} else if err := state.applyResumeLine(trimmed); err != nil {
				// Skip corrupt lines rather than failing the whole resume (P0).
				state.CorruptLines++
				fmt.Fprintf(stdout.Writer(), "[ocr] Warning: skipping corrupt resume line %d in session %q: %v\n", lineNo, sessionID, err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read resume session %q: %w", sessionID, readErr)
		}
	}
	if state.SessionID == "" {
		state.SessionID = sessionID
	}
	return state, nil
}

func bytesTrimSpace(b []byte) []byte {
	// Avoid strings.TrimSpace allocation for the common path; manual trim is fine.
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\r' || b[start] == '\n') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\r' || b[end-1] == '\n') {
		end--
	}
	return b[start:end]
}

func (s *ResumeState) ensureMaps() {
	if s.Items == nil {
		s.Items = make(map[string]ResumeItem)
	}
	if s.FailedFiles == nil {
		s.FailedFiles = make(map[string]string)
	}
	if s.Conversations == nil {
		s.Conversations = make(map[string]ConversationCheckpoint)
	}
	if s.Partials == nil {
		s.Partials = make(map[string]PartialItem)
	}
}

func (s *ResumeState) applyResumeLine(line []byte) error {
	var rec resumeRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return fmt.Errorf("parse resume session %q: %w", s.SessionID, err)
	}
	s.ensureMaps()

	switch rec.Type {
	case "session_start":
		s.applySessionStart(rec)
	case "review_item_done", "review_item_reused":
		if rec.Fingerprint == "" {
			return nil
		}
		filePath := rec.FilePath
		if filePath == "" {
			filePath = rec.NewPath
		}
		// If a previously-done item has zero comments and is not a reuse,
		// treat it as incomplete — the LLM may have returned nothing.
		if rec.Type == "review_item_done" && len(rec.Comments) == 0 {
			s.FailedFiles[rec.Fingerprint] = filePath
			return nil
		}
		delete(s.FailedFiles, rec.Fingerprint)
		delete(s.Conversations, rec.Fingerprint)
		delete(s.Partials, rec.Fingerprint)
		s.Items[rec.Fingerprint] = ResumeItem{
			FilePath:    filePath,
			OldPath:     rec.OldPath,
			NewPath:     rec.NewPath,
			Fingerprint: rec.Fingerprint,
			Comments:    copyLlmComments(rec.Comments),
		}
	case "review_item_failed":
		if rec.Fingerprint != "" {
			delete(s.Items, rec.Fingerprint)
			filePath := rec.FilePath
			if filePath == "" {
				filePath = rec.NewPath
			}
			s.FailedFiles[rec.Fingerprint] = filePath
		}
	case "conversation_checkpoint":
		if rec.Fingerprint == "" || len(rec.Messages) == 0 {
			return nil
		}
		// Later checkpoints supersede earlier ones for the same fingerprint.
		filePath := rec.FilePath
		if filePath == "" {
			filePath = rec.NewPath
		}
		s.Conversations[rec.Fingerprint] = ConversationCheckpoint{
			FilePath:            filePath,
			Fingerprint:         rec.Fingerprint,
			Phase:               rec.Phase,
			PlanGuidance:        rec.PlanGuidance,
			Messages:            copyMessages(rec.Messages),
			Round:               rec.Round,
			Model:               rec.Model,
			TemplateHash:        rec.TemplateHash,
			CommentFingerprints: append([]string(nil), rec.CommentFingerprints...),
			Status:              rec.Status,
			StopReason:          rec.StopReason,
			Comments:            copyLlmComments(rec.Comments),
		}
		// A checkpoint implies the file is not done.
		delete(s.Items, rec.Fingerprint)
		if filePath != "" {
			s.FailedFiles[rec.Fingerprint] = filePath
		}
	case "review_item_partial":
		if rec.Fingerprint == "" {
			return nil
		}
		filePath := rec.FilePath
		if filePath == "" {
			filePath = rec.NewPath
		}
		s.Partials[rec.Fingerprint] = PartialItem{
			FilePath:     filePath,
			Fingerprint:  rec.Fingerprint,
			Phase:        rec.Phase,
			PlanGuidance: rec.PlanGuidance,
			Round:        rec.Round,
			StopReason:   rec.StopReason,
			Comments:     copyLlmComments(rec.Comments),
		}
	case "file_started":
		// Diagnostic only; no state mutation required for resume.
	}
	return nil
}

func (s *ResumeState) applySessionStart(rec resumeRecord) {
	if rec.SessionID != "" {
		s.SessionID = rec.SessionID
	}
	if rec.Cwd != "" {
		s.RepoDir = rec.Cwd
	}
	s.GitBranch = rec.GitBranch
	s.Model = rec.Model
	s.ReviewMode = rec.ReviewMode
	s.DiffFrom = rec.DiffFrom
	s.DiffTo = rec.DiffTo
	s.DiffCommit = rec.DiffCommit
}

// CompletedCount returns the number of reusable file-level checkpoints.
func (s *ResumeState) CompletedCount() int {
	if s == nil {
		return 0
	}
	return len(s.Items)
}

// FailedCount returns the number of failed files that can be retried.
func (s *ResumeState) FailedCount() int {
	if s == nil {
		return 0
	}
	return len(s.FailedFiles)
}

// InProgressCount returns files with a mid-file conversation checkpoint.
func (s *ResumeState) InProgressCount() int {
	if s == nil {
		return 0
	}
	return len(s.Conversations)
}

// HasSessionScope reports whether this resume state knows any per-file work
// from the prior session. When false, callers should treat resume as a fresh
// full enumeration (legacy empty session).
func (s *ResumeState) HasSessionScope() bool {
	if s == nil {
		return false
	}
	return len(s.Items) > 0 || len(s.FailedFiles) > 0 || len(s.Conversations) > 0 || len(s.Partials) > 0
}

// InSession reports whether fingerprint was part of the prior session
// (completed, failed, mid-file checkpoint, or partial findings).
// Full-scan resume (scan/refactor) must only dispatch InSession files so a
// single failed file does not re-trigger a whole-repo run.
func (s *ResumeState) InSession(fingerprint string) bool {
	if s == nil || fingerprint == "" {
		return false
	}
	if _, ok := s.Items[fingerprint]; ok {
		return true
	}
	if _, ok := s.FailedFiles[fingerprint]; ok {
		return true
	}
	if _, ok := s.Conversations[fingerprint]; ok {
		return true
	}
	if _, ok := s.Partials[fingerprint]; ok {
		return true
	}
	return false
}

// NeedsDispatch reports whether fingerprint still requires LLM work on resume
// (not successfully completed).
func (s *ResumeState) NeedsDispatch(fingerprint string) bool {
	if s == nil || fingerprint == "" {
		return false
	}
	if _, ok := s.Items[fingerprint]; ok {
		return false
	}
	return s.InSession(fingerprint)
}

// Item returns a copy of the checkpoint for fingerprint.
func (s *ResumeState) Item(fingerprint string) (ResumeItem, bool) {
	if s == nil {
		return ResumeItem{}, false
	}
	item, ok := s.Items[fingerprint]
	if !ok {
		return ResumeItem{}, false
	}
	item.Comments = copyLlmComments(item.Comments)
	return item, true
}

// Conversation returns the latest conversation checkpoint for fingerprint.
func (s *ResumeState) Conversation(fingerprint string) (ConversationCheckpoint, bool) {
	if s == nil {
		return ConversationCheckpoint{}, false
	}
	cp, ok := s.Conversations[fingerprint]
	if !ok {
		return ConversationCheckpoint{}, false
	}
	cp.Messages = copyMessages(cp.Messages)
	cp.Comments = copyLlmComments(cp.Comments)
	return cp, true
}

// Partial returns the latest partial-findings record for fingerprint.
func (s *ResumeState) Partial(fingerprint string) (PartialItem, bool) {
	if s == nil {
		return PartialItem{}, false
	}
	p, ok := s.Partials[fingerprint]
	if !ok {
		return PartialItem{}, false
	}
	p.Comments = copyLlmComments(p.Comments)
	return p, true
}

// ValidateOptions verifies that the requested review range matches the prior session.
func (s *ResumeState) ValidateOptions(opts SessionOptions) error {
	if s == nil {
		return nil
	}
	if opts.ReviewMode == "" {
		return fmt.Errorf("resume requires --from/--to, --commit, or a full-scan session")
	}
	if s.ReviewMode == "" {
		if opts.ReviewMode == ReviewModeFullScan {
			// Older scan sessions may not have reviewMode metadata; allow resume
			// so the user can retry files that were reviewed before checkpoint
			// recording was added.
			return nil
		}
		return fmt.Errorf("resume session %q is missing review mode metadata", s.SessionID)
	}
	if s.ReviewMode != opts.ReviewMode {
		return fmt.Errorf("resume session review mode %q does not match current mode %q", s.ReviewMode, opts.ReviewMode)
	}
	switch opts.ReviewMode {
	case ReviewModeRange:
		if s.DiffFrom != opts.DiffFrom || s.DiffTo != opts.DiffTo {
			return fmt.Errorf("resume session range %q..%q does not match current range %q..%q", s.DiffFrom, s.DiffTo, opts.DiffFrom, opts.DiffTo)
		}
	case ReviewModeCommit:
		if s.DiffCommit != opts.DiffCommit {
			return fmt.Errorf("resume session commit %q does not match current commit %q", s.DiffCommit, opts.DiffCommit)
		}
	case ReviewModeFullScan:
		// Full-scan fingerprint is path-based; no diff validation needed.
	case ReviewModeWorkspace:
		// Workspace mode fingerprints are path-based; no diff range to match.
	default:
		return fmt.Errorf("resume mode %q is not supported", opts.ReviewMode)
	}
	return nil
}

func copyLlmComments(in []model.LlmComment) []model.LlmComment {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.LlmComment, len(in))
	copy(out, in)
	return out
}

// NormalizeResumeMode returns a supported resume mode (default: continue).
func NormalizeResumeMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case ResumeModeRestartFailed, "restart", "cold":
		return ResumeModeRestartFailed
	default:
		return ResumeModeContinue
	}
}
