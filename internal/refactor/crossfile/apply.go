package crossfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SandboxEnv manages a temporary git worktree for safe apply+verify.
// Changes are applied in the sandbox first; only merged on success.
type SandboxEnv struct {
	repoDir string
	tmpDir  string
}

// NewSandbox creates a temporary directory that mirrors the repo.
// TODO: prefer git worktree or copy-on-write for large repos.
func NewSandbox(repoDir string) (*SandboxEnv, error) {
	tmpDir, err := os.MkdirTemp("", "ocr-sandbox-*")
	if err != nil {
		return nil, fmt.Errorf("create sandbox: %w", err)
	}
	if err := copyDir(repoDir, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir) // clean up on failure
		return nil, fmt.Errorf("sandbox copy: %w", err)
	}
	return &SandboxEnv{repoDir: repoDir, tmpDir: tmpDir}, nil
}

// Dir returns the sandbox directory path.
func (s *SandboxEnv) Dir() string { return s.tmpDir }

// backupEntry holds pre-merge content and mode for rollback.
type mergeBackupEntry struct {
	data []byte
	mode os.FileMode
	// existed tracks whether dst existed before Merge overwrote it.
	// false means the file was newly created (rollback should remove it).
	existed bool
}

// Merge copies all written files back to the original repo and deletes
// files listed in deleted from the repo. On failure, restores pre-merge
// contents for already-processed files and re-creates already-deleted files.
func (s *SandboxEnv) Merge(written []string, deleted []string) error {
	backup := map[string]mergeBackupEntry{} // dst abs path → pre-merge state

	// Process writes: copy sandbox → repo with pre-overwrite backup
	var merged []string
	for _, rel := range written {
		src := filepath.Join(s.tmpDir, filepath.FromSlash(rel))
		dst := filepath.Join(s.repoDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			s.rollbackMerged(backup, merged)
			return fmt.Errorf("merge mkdir %s: %w", rel, err)
		}
		data, err := os.ReadFile(src)
		if err != nil {
			s.rollbackMerged(backup, merged)
			return fmt.Errorf("merge read %s: %w", rel, err)
		}
		// Preserve source file mode instead of hardcoding 0644
		fi, statErr := os.Stat(src)
		mode := os.FileMode(0o644)
		if statErr == nil {
			mode = fi.Mode()
		}
		// Backup pre-merge content if dst already exists
		if _, exists := backup[dst]; !exists {
			if prev, prevErr := os.ReadFile(dst); prevErr == nil {
				fi2, _ := os.Stat(dst)
				pm := os.FileMode(0o644)
				if fi2 != nil {
					pm = fi2.Mode()
				}
				backup[dst] = mergeBackupEntry{data: prev, mode: pm, existed: true}
			} else {
				backup[dst] = mergeBackupEntry{existed: false}
			}
		}
		if err := os.WriteFile(dst, data, mode); err != nil {
			s.rollbackMerged(backup, merged)
			return fmt.Errorf("merge write %s: %w", rel, err)
		}
		merged = append(merged, rel)
	}

	// Process deletes: remove files from the repo
	for _, rel := range deleted {
		dst := filepath.Join(s.repoDir, filepath.FromSlash(rel))
		// Backup before deleting
		if _, exists := backup[dst]; !exists {
			if prev, prevErr := os.ReadFile(dst); prevErr == nil {
				fi2, _ := os.Stat(dst)
				pm := os.FileMode(0o644)
				if fi2 != nil {
					pm = fi2.Mode()
				}
				backup[dst] = mergeBackupEntry{data: prev, mode: pm, existed: true}
			} else {
				// File doesn't exist — nothing to delete, skip
				continue
			}
		}
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			s.rollbackMerged(backup, merged)
			return fmt.Errorf("merge delete %s: %w", rel, err)
		}
		merged = append(merged, rel)
	}

	return nil
}

// rollbackMerged restores pre-merge contents for already-merged files
// (best-effort; errors are logged but not propagated).
func (s *SandboxEnv) rollbackMerged(backup map[string]mergeBackupEntry, merged []string) {
	for _, rel := range merged {
		dst := filepath.Join(s.repoDir, filepath.FromSlash(rel))
		if e, ok := backup[dst]; ok {
			if e.existed {
				_ = os.WriteFile(dst, e.data, e.mode)
			} else {
				_ = os.Remove(dst)
			}
		}
	}
}

// Cleanup removes the sandbox directory.
func (s *SandboxEnv) Cleanup() {
	if s.tmpDir != "" {
		if err := os.RemoveAll(s.tmpDir); err != nil {
			// Log error; caller cannot recover from cleanup failure
			_ = fmt.Errorf("sandbox cleanup: %w", err)
		}
	}
}

// copyDir copies all files from src to dst.
// Skips .git directory. Handles symlinks by copying the target.
func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == ".git" {
			continue
		}
		srcPath := filepath.Join(src, name)
		dstPath := filepath.Join(dst, name)
		if e.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else if e.Type()&os.ModeSymlink != 0 {
			// Copy symlink target as regular file (sandbox is temporary)
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, 0o644); err != nil {
				return err
			}
		} else {
			data, err := os.ReadFile(srcPath)
			if err != nil {
				return err
			}
			if err := os.WriteFile(dstPath, data, e.Type().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

type backupEntry struct {
	data []byte
	mode os.FileMode
}

// ApplyResult summarizes a multi-file apply attempt.
type ApplyResult struct {
	Written      []string
	Deleted      []string // files deleted from the repo (action="delete")
	Skipped      []string
	Verify       VerifyResult
	RolledBack   bool
	Messages     []string
	AppliedCount int // number of suggestion_code items actually applied (not file count)
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
				res.Deleted = append(res.Deleted, st.Path)
				// deleted files are not added to Written — they can't be verified by read-back
				continue
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
