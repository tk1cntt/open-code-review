// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
	"github.com/alibaba/open-code-review/internal/stdout"
)

// ResumeState is the replayed, read-only checkpoint index for one prior session.
type ResumeState struct {
	SessionID        string
	RepoDir          string
	GitBranch        string
	Model            string
	ReviewMode       string
	DiffFrom         string
	DiffTo           string
	DiffCommit       string
	ScanPaths        []string
	HasScanPathScope bool
	Items            map[string]ResumeItem
	// FailedFiles maps fingerprint → newPath for files that failed in the
	// previous session, so callers can log which files are being retried.
	FailedFiles map[string]string
	// Conversations maps fingerprint → latest mid-file conversation checkpoint.
	Conversations map[string]ConversationCheckpoint
	// Partials maps fingerprint → latest review_item_partial record.
	Partials map[string]PartialItem
	// CorruptLines is the number of JSONL lines skipped during a lenient load
	// (LoadReviewResumeState). Strict LoadResumeState fails instead of skipping.
	CorruptLines int

	// Manifest is the parent run's coverage snapshot, carried only by the
	// session_end record. A nil Manifest means the parent's input identity cannot
	// be verified, not that the parent did no work.
	Manifest *RunManifest

	// Closed reports whether a session_end record was replayed. A session can
	// close without a manifest — legacy sessions predate manifests, and a run that
	// never froze one still writes session_end — so Closed is what tells an
	// interrupted parent apart from one that closed with nothing to verify.
	Closed bool

	// reusable caches the parent manifest's completed and reused fingerprints,
	// built on first use by ReusableItem. Reuse is decided on one goroutine
	// before any dispatch begins, so this needs no lock.
	reusable map[string]bool
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
	ScanPaths           *[]string          `json:"scanPaths"`
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
	RunManifest         *RunManifest       `json:"run_manifest"`
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

// LoadResumeState replays a previous session JSONL into a fingerprint index. A
// record that cannot be parsed fails the load: with nothing to arbitrate coverage,
// a dropped line is indistinguishable from a checkpoint that was never written,
// and the pair it may have belonged to — a review_item_failed retracting an
// earlier done record — cannot be reconstructed from the rest of the file.
func LoadResumeState(repoDir, sessionID string) (*ResumeState, error) {
	return loadResumeState(repoDir, sessionID, false)
}

// LoadReviewResumeState replays a review session, dropping records it cannot
// parse. Review reuse is gated on the parent manifest rather than on these lines
// (see ReusableItem), so an unreadable checkpoint just means its file is reviewed
// again — which is what a corrupted checkpoint is supposed to do. Failing the
// whole load instead would turn one bad line into the loss of every other file's
// checkpoint.
func LoadReviewResumeState(repoDir, sessionID string) (*ResumeState, error) {
	return loadResumeState(repoDir, sessionID, true)
}

func loadResumeState(repoDir, sessionID string, skipUnparseable bool) (*ResumeState, error) {
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
				if !skipUnparseable {
					return nil, err
				}
				// Skip corrupt lines rather than failing the whole review resume.
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

// applyResumeLine folds one record into the index. It reports an unparseable
// line to the caller, which decides whether that is fatal.
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
	case "session_end":
		s.Closed = true
		// Last one wins: a session file holds at most one session_end, but
		// replaying a truncated write should not clear an earlier good one.
		if rec.RunManifest != nil {
			s.Manifest = rec.RunManifest
		}
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
	if rec.ScanPaths != nil {
		s.ScanPaths = normalizeScanPaths(*rec.ScanPaths)
		s.HasScanPathScope = true
	}
}

// CompletedCount returns the number of file-level checkpoints replay recovered.
// Review reuse is narrower — see ReusableItem — while scan, having no manifest
// to consult, reuses exactly these.
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

// ReusableItem returns the checkpoint for fingerprint only if the parent
// manifest also recorded that fingerprint as completed or reused.
//
// The manifest is the single source of truth for coverage, so a checkpoint line
// alone is not enough: the parent froze its verdict per item, and a replayed
// record that the manifest does not vouch for is a record whose outcome the
// parent did not stand behind. This is also what makes a dropped
// review_item_failed line harmless here — the failure is recorded in the
// manifest too, and coverage never lies about it.
func (s *ResumeState) ReusableItem(fingerprint string) (ResumeItem, bool) {
	if s == nil || s.Manifest == nil {
		return ResumeItem{}, false
	}
	if s.reusable == nil {
		s.reusable = manifestReusableFingerprints(s.Manifest)
	}
	if !s.reusable[fingerprint] {
		return ResumeItem{}, false
	}
	if item, ok := s.Item(fingerprint); ok {
		return item, true
	}
	// A completed/reused manifest item with zero comments is parked in
	// FailedFiles by applyResumeLine so scan (no manifest) still retries
	// empty "done" records. Trust the coverage snapshot: no-findings is a
	// valid completed review.
	if path, ok := s.FailedFiles[fingerprint]; ok {
		return ResumeItem{FilePath: path, NewPath: path, Fingerprint: fingerprint}, true
	}
	return ResumeItem{}, false
}

// manifestReusableFingerprints collects the fingerprints the parent manifest
// settled as completed or reused. Both count: a parent that itself resumed
// carries forward results it did not compute, and those are no less final.
func manifestReusableFingerprints(m *RunManifest) map[string]bool {
	out := make(map[string]bool, len(m.Coverage.Completed)+len(m.Coverage.Reused))
	for _, group := range [][]CoverageItem{m.Coverage.Completed, m.Coverage.Reused} {
		for _, item := range group {
			if item.Fingerprint != "" {
				out[item.Fingerprint] = true
			}
		}
	}
	return out
}

// ValidateOptions verifies that this session can be resumed in the requested
// review mode at all. It deliberately does not compare the ref text the user
// typed: `abc1234` and `abc1234def` can name the same commit while a ref whose
// name did not change can name a new one, so ref spellings are neither
// sufficient nor necessary evidence about the input. ValidateResume compares the
// resolved input identity instead.
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
	case ReviewModeRange, ReviewModeCommit:
		// Ref text is not evidence about the input; ValidateResume compares identity.
	case ReviewModeFullScan:
		// Full-scan fingerprint is path-based; path scope is ValidateScanOptions.
	case ReviewModeWorkspace:
		// Workspace mode fingerprints are path-based; no diff range to match.
	default:
		return fmt.Errorf("resume mode %q is not supported", opts.ReviewMode)
	}
	return nil
}

// ValidateScanOptions verifies that the previous session was a full-file scan.
func (s *ResumeState) ValidateScanOptions(scanPaths []string) error {
	if s == nil {
		return nil
	}
	if s.ReviewMode == "" {
		return fmt.Errorf("resume session %q is missing review mode metadata", s.SessionID)
	}
	if s.ReviewMode != ReviewModeFullScan {
		return fmt.Errorf("resume session review mode %q does not match current mode %q", s.ReviewMode, ReviewModeFullScan)
	}
	current := normalizeScanPaths(scanPaths)
	if s.HasScanPathScope && !equalStringSlices(s.ScanPaths, current) {
		return fmt.Errorf("resume session scan path scope %q does not match current scope %q", formatScanScope(s.ScanPaths), formatScanScope(current))
	}
	return nil
}

func normalizeScanPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		p = strings.TrimSpace(p)
		p = strings.TrimPrefix(p, "./")
		p = strings.TrimSuffix(filepath.ToSlash(p), "/")
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func formatScanScope(paths []string) string {
	if len(paths) == 0 {
		return "<whole repo>"
	}
	return strings.Join(paths, ",")
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
