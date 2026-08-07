package llmloop

import (
	"github.com/alibaba/open-code-review/internal/session"
)

// BindSessionCheckpoint installs a CheckpointHook that writes conversation
// checkpoints to the session JSONL after each successful main-loop round.
// planGuidance/model/templateHash are frozen for this file's run.
func BindSessionCheckpoint(r *Runner, sh *session.SessionHistory, fingerprint, planGuidance, model, templateHash string) {
	if r == nil || sh == nil || fingerprint == "" {
		return
	}
	r.SetCheckpointHook(func(info RoundCheckpointInfo) {
		cp := session.ConversationCheckpoint{
			FilePath:     info.FilePath,
			Fingerprint:  fingerprint,
			Phase:        session.PhaseMain,
			PlanGuidance: planGuidance,
			Messages:     info.Messages,
			Round:        info.Round,
			Model:        model,
			TemplateHash: templateHash,
			Status:       session.CheckpointInProgress,
		}
		if r.deps.CommentCollector != nil {
			cp.Comments = r.deps.CommentCollector.CommentsForPath(info.FilePath)
			cp.CommentFingerprints = session.CommentFingerprints(cp.Comments)
		}
		sh.SaveConversationCheckpoint(cp)
	})
}

// ClearCheckpointHook removes any mid-file checkpoint callback and resets
// the completed-rounds offset for the next file.
func ClearCheckpointHook(r *Runner) {
	if r != nil {
		r.SetCheckpointHook(nil)
		r.SetCompletedRounds(0)
	}
}
