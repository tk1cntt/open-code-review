package crossfile

import (
	"hash/fnv"
	"strings"
	"unicode"
)

// FunctionWindow is a normalized body window for similarity.
type FunctionWindow struct {
	Path      string
	Name      string
	StartLine int
	EndLine   int
	Norm      string
	Shingles  map[uint64]struct{}
}

// ExtractWindows splits content into coarse function-like windows (lang-agnostic).
func ExtractWindows(path, content string) []FunctionWindow {
	lines := strings.Split(content, "\n")
	var windows []FunctionWindow
	var curStart int
	var curName string
	var cur []string

	flush := func(end int) {
		if len(cur) < 4 {
			cur = nil
			return
		}
		body := strings.Join(cur, "\n")
		norm := NormalizeCode(body)
		if len(norm) < 40 {
			cur = nil
			return
		}
		windows = append(windows, FunctionWindow{
			Path:      path,
			Name:      curName,
			StartLine: curStart + 1,
			EndLine:   end,
			Norm:      norm,
			Shingles:  ShingleSet(norm, 5),
		})
		cur = nil
	}

	for i, line := range lines {
		if isFuncStart(line) {
			if len(cur) > 0 {
				flush(i)
			}
			curStart = i
			curName = funcNameFromLine(line)
			cur = []string{line}
			continue
		}
		if cur != nil {
			cur = append(cur, line)
			// Split large windows to keep shingle computation bounded.
			// A cap of 120 lines prevents memory spikes on huge functions;
			// similarity detection across split windows is approximate.
			if len(cur) > 120 {
				flush(i + 1)
			}
		}
	}
	if len(cur) > 0 {
		flush(len(lines))
	}

	// Fallback: whole file as one window if nothing found and file medium-sized
	if len(windows) == 0 && len(lines) >= 8 && len(lines) <= 200 {
		norm := NormalizeCode(content)
		windows = append(windows, FunctionWindow{
			Path: path, Name: "_file", StartLine: 1, EndLine: len(lines),
			Norm: norm, Shingles: ShingleSet(norm, 5),
		})
	}
	return windows
}

func isFuncStart(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	prefixes := []string{"func ", "def ", "async def ", "function ", "export function ",
		"export async function ", "public ", "private ", "protected ", "fn "}
	for _, p := range prefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	// const foo = (
	if strings.HasPrefix(t, "const ") && strings.Contains(t, "=") && strings.Contains(t, "(") {
		return true
	}
	return false
}

func funcNameFromLine(line string) string {
	t := strings.TrimSpace(line)
	fields := strings.Fields(t)
	for i, f := range fields {
		if f == "func" || f == "def" || f == "function" || f == "fn" || f == "async" {
			if i+1 < len(fields) {
				name := fields[i+1]
				name = strings.TrimSuffix(name, "(")
				if name != "" && name != "def" && name != "function" {
					return name
				}
			}
		}
		if f == "const" && i+1 < len(fields) {
			return strings.TrimSuffix(fields[i+1], "=")
		}
	}
	if len(fields) > 0 {
		return fields[0]
	}
	return "anon"
}

// NormalizeCode strips comments-ish noise and collapses whitespace/idents lightly.
func NormalizeCode(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		prevSpace = false
		// drop string quotes content coarsely: keep placeholder
		b.WriteRune(unicode.ToLower(r))
	}
	out := b.String()
	// remove line comments
	parts := strings.Split(out, "\n")
	var cleaned []string
	for _, p := range parts {
		if i := strings.Index(p, "//"); i >= 0 {
			p = p[:i]
		}
		if i := strings.Index(p, "#"); i >= 0 && !strings.Contains(p, "#!") {
			// careful: keep python shebang only; still ok for similarity
		}
		cleaned = append(cleaned, strings.TrimSpace(p))
	}
	return strings.Join(cleaned, " ")
}

// ShingleSet returns character n-gram hashes.
func ShingleSet(s string, n int) map[uint64]struct{} {
	if n < 2 {
		n = 3
	}
	set := make(map[uint64]struct{})
	if len(s) < n {
		h := fnv.New64a()
		_, _ = h.Write([]byte(s))
		set[h.Sum64()] = struct{}{}
		return set
	}
	for i := 0; i+n <= len(s); i++ {
		h := fnv.New64a()
		_, _ = h.Write([]byte(s[i : i+n]))
		set[h.Sum64()] = struct{}{}
	}
	return set
}

// Jaccard returns similarity in [0,1].
func Jaccard(a, b map[uint64]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// SimilarPairs finds cross-file window pairs with score >= minScore.
func SimilarPairs(windows []FunctionWindow, minScore float64) []struct {
	A, B  FunctionWindow
	Score float64
} {
	var pairs []struct {
		A, B  FunctionWindow
		Score float64
	}
	// O(n^2) pairwise comparison; ensure window count is bounded by upstream filters.
	for i := 0; i < len(windows); i++ {
		for j := i + 1; j < len(windows); j++ {
			if windows[i].Path == windows[j].Path {
				continue
			}
			sc := Jaccard(windows[i].Shingles, windows[j].Shingles)
			if sc >= minScore {
				pairs = append(pairs, struct {
					A, B  FunctionWindow
					Score float64
				}{windows[i], windows[j], sc})
			}
		}
	}
	return pairs
}
