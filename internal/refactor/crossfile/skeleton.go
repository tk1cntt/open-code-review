package crossfile

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reGoFunc    = regexp.MustCompile(`(?m)^func\s+(?:\([^)]+\)\s+)?([A-Za-z_][\w]*)\s*\([^;{]*\)[^{]*\{?`)
	reGoType    = regexp.MustCompile(`(?m)^type\s+([A-Za-z_][\w]*)\s+`)
	rePyDef     = regexp.MustCompile(`(?m)^(async\s+)?def\s+([A-Za-z_][\w]*)\s*\([^)]*\)\s*:`)
	rePyClass   = regexp.MustCompile(`(?m)^class\s+([A-Za-z_][\w]*)\s*[\(:]`)
	reJSFunc    = regexp.MustCompile(`(?m)^(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][\w]*)\s*\(`)
	reJSConstFn = regexp.MustCompile(`(?m)^(?:export\s+)?const\s+([A-Za-z_][\w]*)\s*=\s*(?:async\s*)?\(`)
	reJSClass   = regexp.MustCompile(`(?m)^(?:export\s+)?class\s+([A-Za-z_][\w]*)`)
	reJavaMethod = regexp.MustCompile(`(?m)^\s*(?:public|private|protected|static|final|synchronized|native|abstract|\s)*\s*[\w.<>,\[\]]+\s+([A-Za-z_][\w]*)\s*\([^;]*\)\s*(?:throws\s+[\w.,\s]+)?\s*\{?`)
	reJavaClass  = regexp.MustCompile(`(?m)^\s*(?:public|private|protected)?\s*(?:static\s+)?(?:final\s+)?(?:class|interface|enum|record)\s+([A-Za-z_][\w]*)`)
)

// ExtractSkeleton returns a compact signature map for a file (Layer 1b).
func ExtractSkeleton(path, content string) string {
	ext := strings.ToLower(filepath.Ext(path))
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", filepath.ToSlash(path))

	// Always include import summary (first 30 import-like lines).
	imports := 0
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if isImportLine(line, ext) {
			fmt.Fprintf(&b, "import %s\n", truncate(line, 120))
			imports++
			if imports >= 30 {
				break
			}
		}
	}
	// Errors can occur with unusually long lines; non-critical for skeleton.
	_ = sc.Err()

	switch ext {
	case ".go":
		for _, m := range reGoType.FindAllStringSubmatch(content, -1) {
			fmt.Fprintf(&b, "type %s\n", m[1])
		}
		for _, m := range reGoFunc.FindAllStringSubmatch(content, -1) {
			// full match line-ish
			sig := firstLineContaining(content, "func "+m[1])
			if sig == "" {
				sig = "func " + m[1] + "(...)"
			}
			fmt.Fprintf(&b, "%s\n", truncate(sig, 160))
		}
	case ".py":
		for _, m := range rePyClass.FindAllStringSubmatch(content, -1) {
			fmt.Fprintf(&b, "class %s\n", m[1])
		}
		for _, m := range rePyDef.FindAllStringSubmatch(content, -1) {
			name := m[2]
			sig := firstLineContaining(content, "def "+name)
			if sig == "" {
				sig = "def " + name + "(...)"
			}
			fmt.Fprintf(&b, "%s\n", truncate(sig, 160))
		}
	case ".ts", ".tsx", ".js", ".jsx":
		for _, m := range reJSClass.FindAllStringSubmatch(content, -1) {
			fmt.Fprintf(&b, "class %s\n", m[1])
		}
		for _, m := range reJSFunc.FindAllStringSubmatch(content, -1) {
			fmt.Fprintf(&b, "function %s(...)\n", m[1])
		}
		for _, m := range reJSConstFn.FindAllStringSubmatch(content, -1) {
			fmt.Fprintf(&b, "const %s = (...)\n", m[1])
		}
	case ".java", ".kt":
		for _, m := range reJavaClass.FindAllStringSubmatch(content, -1) {
			fmt.Fprintf(&b, "type %s\n", m[1])
		}
		for _, m := range reJavaMethod.FindAllStringSubmatch(content, -1) {
			if m[1] == "if" || m[1] == "for" || m[1] == "while" || m[1] == "switch" {
				continue
			}
			fmt.Fprintf(&b, "method %s(...)\n", m[1])
		}
	default:
		// Fallback: non-empty lines that look like definitions
		for _, line := range strings.Split(content, "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "func ") || strings.HasPrefix(t, "def ") ||
				strings.HasPrefix(t, "class ") || strings.HasPrefix(t, "function ") {
				fmt.Fprintf(&b, "%s\n", truncate(t, 160))
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func isImportLine(line, ext string) bool {
	switch ext {
	case ".go":
		return strings.HasPrefix(line, "import ") || (strings.HasPrefix(line, "\"") && strings.HasSuffix(line, "\""))
	case ".py":
		return strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ")
	case ".java", ".kt":
		return strings.HasPrefix(line, "import ")
	default:
		return strings.HasPrefix(line, "import ") || strings.Contains(line, " from ")
	}
}

func firstLineContaining(content, needle string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
