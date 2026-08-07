package session

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/alibaba/open-code-review/internal/llm"
	"github.com/alibaba/open-code-review/internal/model"
)

// File start modes for mid-file resume.
const (
	ModeCold     = "cold"
	ModeContinue = "continue"
	ModeReuse    = "reuse"
)

// ResumeMode controls how --resume treats mid-file checkpoints.
const (
	ResumeModeContinue      = "continue"
	ResumeModeRestartFailed = "restart-failed"
)

// Conversation checkpoint status values.
const (
	CheckpointInProgress = "in_progress"
	CheckpointTimedOut   = "timed_out"
	CheckpointFailed     = "failed"
)

// Conversation phases.
const (
	PhasePlan = "plan"
	PhaseMain = "main"
)

// ConversationCheckpoint is a mid-file recovery snapshot after a successful
// tool-use round. Messages are the full transcript ready for RunPerFile.
type ConversationCheckpoint struct {
	FilePath            string            `json:"filePath"`
	Fingerprint         string            `json:"fingerprint"`
	Phase               string            `json:"phase"` // plan | main
	PlanGuidance        string            `json:"planGuidance,omitempty"`
	Messages            []llm.Message     `json:"messages"`
	Round               int               `json:"round"`
	Model               string            `json:"model,omitempty"`
	TemplateHash        string            `json:"templateHash,omitempty"`
	CommentFingerprints []string          `json:"commentFingerprints,omitempty"`
	Status              string            `json:"status"` // in_progress | timed_out | failed
	StopReason          string            `json:"stopReason,omitempty"`
	Comments            []model.LlmComment `json:"comments,omitempty"`
}

// FileStart describes how to start work on a single file under resume.
type FileStart struct {
	Mode         string // ModeCold | ModeContinue | ModeReuse
	Messages     []llm.Message
	PlanGuidance string
	SeedComments []model.LlmComment
	Round        int
	Checkpoint   ConversationCheckpoint
	ReusedItem   ResumeItem
}

// PrepareOpts controls PrepareFileStart validation and mode selection.
type PrepareOpts struct {
	// ResumeMode is continue (default) or restart-failed.
	ResumeMode string
	// CurrentModel is the model for this run; mismatch invalidates continue.
	CurrentModel string
	// TemplateHash is the current template hash; mismatch invalidates continue.
	TemplateHash string
	// StrictModel when true treats empty current/checkpoint model as mismatch.
	StrictModel bool
}

// PrepareFileStart decides reuse / mid-file continue / cold start for one fingerprint.
func PrepareFileStart(resume *ResumeState, fingerprint, path string, opts PrepareOpts) FileStart {
	if resume == nil {
		return FileStart{Mode: ModeCold}
	}
	if item, ok := resume.Item(fingerprint); ok {
		return FileStart{Mode: ModeReuse, ReusedItem: item, SeedComments: copyLlmComments(item.Comments)}
	}

	mode := strings.TrimSpace(opts.ResumeMode)
	if mode == "" {
		mode = ResumeModeContinue
	}
	if mode == ResumeModeRestartFailed {
		return FileStart{Mode: ModeCold}
	}

	cp, ok := resume.Conversation(fingerprint)
	if !ok || len(cp.Messages) == 0 {
		// Fall back to partial comments only (Phase 1).
		if partial, pok := resume.Partial(fingerprint); pok && len(partial.Comments) > 0 {
			return FileStart{
				Mode:         ModeCold,
				PlanGuidance: partial.PlanGuidance,
				SeedComments: copyLlmComments(partial.Comments),
				Round:        partial.Round,
			}
		}
		return FileStart{Mode: ModeCold}
	}

	if !checkpointValid(cp, opts) {
		// Invalid transcript, but partial comments may still seed a cold start.
		if len(cp.Comments) > 0 {
			return FileStart{
				Mode:         ModeCold,
				PlanGuidance: cp.PlanGuidance,
				SeedComments: copyLlmComments(cp.Comments),
				Round:        cp.Round,
			}
		}
		if partial, pok := resume.Partial(fingerprint); pok && len(partial.Comments) > 0 {
			return FileStart{
				Mode:         ModeCold,
				PlanGuidance: partial.PlanGuidance,
				SeedComments: copyLlmComments(partial.Comments),
				Round:        partial.Round,
			}
		}
		return FileStart{Mode: ModeCold}
	}

	if cp.FilePath == "" {
		cp.FilePath = path
	}
	return FileStart{
		Mode:         ModeContinue,
		Messages:     copyMessages(cp.Messages),
		PlanGuidance: cp.PlanGuidance,
		SeedComments: copyLlmComments(cp.Comments),
		Round:        cp.Round,
		Checkpoint:   cp,
	}
}

func checkpointValid(cp ConversationCheckpoint, opts PrepareOpts) bool {
	if len(cp.Messages) == 0 {
		return false
	}
	// Permanent non-retryable terminal status must not continue.
	if cp.Status == CheckpointFailed && isNonRetryableStop(cp.StopReason) {
		return false
	}
	if opts.TemplateHash != "" && cp.TemplateHash != "" && opts.TemplateHash != cp.TemplateHash {
		return false
	}
	if opts.CurrentModel != "" && cp.Model != "" && opts.CurrentModel != cp.Model {
		return false
	}
	if opts.StrictModel {
		if opts.CurrentModel == "" || cp.Model == "" || opts.CurrentModel != cp.Model {
			return false
		}
	}
	return true
}

func isNonRetryableStop(reason string) bool {
	r := strings.ToLower(reason)
	return strings.Contains(r, "task failed") ||
		strings.Contains(r, "401") ||
		strings.Contains(r, "unauthorized") ||
		strings.Contains(r, "invalid api key") ||
		strings.Contains(r, "authentication")
}

// CommentFingerprint returns a stable dedupe key for a comment.
// Uses path + content hash (not line) so the same finding re-emitted at a
// slightly different line does not duplicate.
func CommentFingerprint(cm model.LlmComment) string {
	h := sha256.Sum256([]byte(cm.Content))
	return fmt.Sprintf("%s:%s", cm.Path, hex.EncodeToString(h[:8]))
}

// CommentFingerprints returns fingerprints for a comment slice.
func CommentFingerprints(comments []model.LlmComment) []string {
	if len(comments) == 0 {
		return nil
	}
	out := make([]string, 0, len(comments))
	seen := make(map[string]struct{}, len(comments))
	for _, cm := range comments {
		fp := CommentFingerprint(cm)
		if _, ok := seen[fp]; ok {
			continue
		}
		seen[fp] = struct{}{}
		out = append(out, fp)
	}
	return out
}

// ComputeTemplateHash builds a short hash of template-relevant fields used
// for checkpoint invalidation (model identity is separate).
func ComputeTemplateHash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// TemplateHasher abstracts the fields relevant for template identity across the
// three template types (Template, ScanTemplate, RefactorTemplate).
type TemplateHasher interface {
	TemplateHashFields() []string
}

// ComputeTemplateHashFrom delegates to TemplateHasher and returns the hash.
func ComputeTemplateHashFrom(th TemplateHasher) string {
	return ComputeTemplateHash(th.TemplateHashFields()...)
}

// FormatResumeSummary builds the user-visible resume line.
func FormatResumeSummary(sessionID string, reused, continuing, coldRetry, newFiles int64) string {
	return fmt.Sprintf(
		"[ocr] Resume %s: reusing %d done, continuing %d in-progress (mid-file), retrying %d cold, %d new",
		sessionID, reused, continuing, coldRetry, newFiles,
	)
}
