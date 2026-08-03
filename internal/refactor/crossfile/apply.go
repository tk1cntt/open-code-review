package crossfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type backupEntry struct {
	data []byte
	mode os.FileMode
}

// ApplyResult summarizes a multi-file apply attempt.
type ApplyResult struct {
	Written  []string
	Skipped  []string
	Verify   VerifyResult
	RolledBack bool
	Messages []string
}

// ApplyPlanSteps writes suggestion_code from plan steps under repoDir.
// On verify failure, restores previous content for written files.
// runTests enables go test in verify for Go packages.
func ApplyPlanSteps(repoDir string, plans []RefactorPlan, runTests bool) ApplyResult {
	res := ApplyResult{}
	backup := map[string]backupEntry{} // abs path → previous content and mode (nil content if new)

	rollback := func() {
		for abs, e := range backup {
			if e.data == nil {
				_ = os.Remove(abs)
			} else {
				_ = os.WriteFile(abs, e.data, e.mode)
			}
		}
		res.RolledBack = true
	}

	for _, plan := range plans {
		steps := append([]PlanStep(nil), plan.Steps...)
		sortSteps(steps)
		for _, st := range steps {
			if st.Path == "" {
				res.Skipped = append(res.Skipped, "(empty path)")
				continue
			}
			if st.SuggestionCode == "" && st.Action != "delete" {
				res.Skipped = append(res.Skipped, st.Path+": no suggestion_code")
				continue
			}
			abs := filepath.Join(repoDir, filepath.FromSlash(st.Path))
			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				res.Messages = append(res.Messages, err.Error())
				rollback()
				res.Verify = VerifyResult{OK: false, Messages: res.Messages}
				return res
			}
			if _, exists := backup[abs]; !exists {
				prev, err := os.ReadFile(abs)
				if err != nil {
					if !os.IsNotExist(err) {
						res.Messages = append(res.Messages, err.Error())
						rollback()
						res.Verify = VerifyResult{OK: false, Messages: res.Messages}
						return res
					}
					backup[abs] = backupEntry{data: nil}
				} else {
					fi, _ := os.Stat(abs)
					mode := os.FileMode(0o644)
					if fi != nil {
						mode = fi.Mode()
					}
					backup[abs] = backupEntry{data: prev, mode: mode}
				}
			}
			if st.Action == "delete" {
				_ = os.Remove(abs)
			} else {
				content := st.SuggestionCode
				if !strings.HasSuffix(content, "\n") {
					content += "\n"
				}
				if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
					res.Messages = append(res.Messages, err.Error())
					rollback()
					res.Verify = VerifyResult{OK: false, Messages: res.Messages}
					return res
				}
			}
			res.Written = append(res.Written, st.Path)
		}
	}

	if len(res.Written) == 0 {
		res.Messages = append(res.Messages, "nothing applied (no suggestion_code on plan steps)")
		res.Verify = VerifyResult{OK: true, Messages: res.Messages}
		return res
	}

	v := VerifyFiles(repoDir, res.Written, runTests)
	res.Verify = v
	if !v.OK {
		res.Messages = append(res.Messages, "verification failed; rolling back")
		res.Messages = append(res.Messages, v.Messages...)
		rollback()
		return res
	}
	res.Messages = append(res.Messages, fmt.Sprintf("applied %d file(s); verify ok", len(res.Written)))
	res.Messages = append(res.Messages, v.Messages...)
	return res
}

// MaxRepairAttempts is the golden-rule self-correct limit.
const MaxRepairAttempts = 3
