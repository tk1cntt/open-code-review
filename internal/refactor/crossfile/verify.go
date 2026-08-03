package crossfile

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// VerifyFiles runs Layer-3 checks on files under repoDir (relative paths).
// For .go files: parse AST. Optionally run `go test` packages when runTests.
func VerifyFiles(repoDir string, relPaths []string, runTests bool) VerifyResult {
	var msgs []string
	ok := true
	pkgDirs := map[string]struct{}{}

	for _, rel := range relPaths {
		rel = filepath.ToSlash(rel)
		abs := filepath.Join(repoDir, filepath.FromSlash(rel))
		data, err := os.ReadFile(abs)
		if err != nil {
			ok = false
			msgs = append(msgs, fmt.Sprintf("read %s: %v", rel, err))
			continue
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			ok = false
			msgs = append(msgs, fmt.Sprintf("%s: empty file", rel))
			continue
		}
		ext := strings.ToLower(filepath.Ext(rel))
		switch ext {
		case ".go":
			fset := token.NewFileSet()
			if _, err := parser.ParseFile(fset, abs, data, parser.AllErrors); err != nil {
				ok = false
				msgs = append(msgs, fmt.Sprintf("go parse %s: %v", rel, err))
			} else {
				msgs = append(msgs, fmt.Sprintf("go parse ok: %s", rel))
			}
			pkgDirs[filepath.ToSlash(filepath.Dir(rel))] = struct{}{}
		default:
			// Generic: balanced braces heuristic for C-like; always pass if non-empty
			if !bracesBalanced(string(data)) {
				msgs = append(msgs, fmt.Sprintf("warning: unbalanced braces in %s", rel))
				// do not hard-fail non-go
			} else {
				msgs = append(msgs, fmt.Sprintf("basic check ok: %s", rel))
			}
		}
	}

	if runTests && ok && len(pkgDirs) > 0 {
		for dir := range pkgDirs {
			pkg := "./" + dir
			if dir == "" || dir == "." {
				pkg = "."
			}
			cmd := exec.Command("go", "test", pkg)
			cmd.Dir = repoDir
			out, err := cmd.CombinedOutput()
			if err != nil {
				ok = false
				msgs = append(msgs, fmt.Sprintf("go test %s: %v\n%s", pkg, err, truncate(string(out), 2000)))
			} else {
				msgs = append(msgs, fmt.Sprintf("go test ok: %s", pkg))
			}
		}
	}

	return VerifyResult{OK: ok, Messages: msgs}
}

func bracesBalanced(s string) bool {
	bal := 0
	for _, r := range s {
		switch r {
		case '{':
			bal++
		case '}':
			bal--
			if bal < 0 {
				return false
			}
		}
	}
	return bal == 0
}
