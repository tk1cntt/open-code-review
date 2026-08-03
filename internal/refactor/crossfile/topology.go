package crossfile

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Topology is a lightweight import/dir graph (Layer 1a).
type Topology struct {
	Nodes []string          `json:"nodes"`
	Edges []TopologyEdge    `json:"edges"`
	// Adjacency: from path → imported paths (resolved within the file set only).
	Adj map[string][]string `json:"-"`
}

// TopologyEdge is a directed import-ish edge.
type TopologyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // import | same_dir
}

var (
	reGoImportLine  = regexp.MustCompile(`(?m)^\s*(?:import\s+)?(?:"([^"]+)"|` + "`" + `([^` + "`" + `]+)` + "`" + `)\s*$`)
	reGoImportBlock = regexp.MustCompile(`(?s)import\s*\((.*?)\)`)
	reTSImportFrom  = regexp.MustCompile(`(?m)from\s+['"]([^'"]+)['"]`)
	reTSImportSide  = regexp.MustCompile(`(?m)import\s+['"]([^'"]+)['"]`)
	rePyImport      = regexp.MustCompile(`(?m)^\s*(?:from\s+([\w.]+)\s+import|import\s+([\w.]+))`)
	reJavaImport    = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([\w.]+)\s*;`)
)

// BuildTopology constructs a graph among the given files only.
func BuildTopology(files []FileInput) *Topology {
	byPath := make(map[string]FileInput, len(files))
	nodes := make([]string, 0, len(files))
	for _, f := range files {
		p := filepath.ToSlash(f.Path)
		byPath[p] = FileInput{Path: p, Content: f.Content}
		nodes = append(nodes, p)
	}
	sort.Strings(nodes)

	// Index basenames and package-ish keys for weak resolution.
	baseIndex := map[string][]string{}
	for _, p := range nodes {
		base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		baseIndex[base] = append(baseIndex[base], p)
	}

	t := &Topology{
		Nodes: nodes,
		Adj:   make(map[string][]string, len(nodes)),
	}

	for _, p := range nodes {
		f := byPath[p]
		rawImports := extractImportStrings(p, f.Content)
		seen := map[string]struct{}{}
		for _, imp := range rawImports {
			targets := resolveImport(p, imp, nodes, baseIndex)
			for _, to := range targets {
				if to == p {
					continue
				}
				key := p + "->" + to
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				t.Edges = append(t.Edges, TopologyEdge{From: p, To: to, Kind: "import"})
				t.Adj[p] = append(t.Adj[p], to)
			}
		}
		// same_dir soft edges for clustering seed (not import).
		dir := filepath.ToSlash(filepath.Dir(p))
		for _, other := range nodes {
			if other == p {
				continue
			}
			if filepath.ToSlash(filepath.Dir(other)) == dir {
				t.Edges = append(t.Edges, TopologyEdge{From: p, To: other, Kind: "same_dir"})
			}
		}
	}
	return t
}

func extractImportStrings(path, content string) []string {
	ext := strings.ToLower(filepath.Ext(path))
	var out []string
	switch ext {
	case ".go":
		if m := reGoImportBlock.FindStringSubmatch(content); len(m) > 1 {
			for _, line := range strings.Split(m[1], "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "//") {
					continue
				}
				// "path" or alias "path"
				if i := strings.Index(line, "\""); i >= 0 {
					if j := strings.Index(line[i+1:], "\""); j >= 0 {
						out = append(out, line[i+1:i+1+j])
					}
				}
			}
		}
		// single-line import "x"
		for _, m := range reGoImportLine.FindAllStringSubmatch(content, -1) {
			if m[1] != "" {
				out = append(out, m[1])
			} else if m[2] != "" {
				out = append(out, m[2])
			}
		}
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		for _, m := range reTSImportFrom.FindAllStringSubmatch(content, -1) {
			out = append(out, m[1])
		}
		for _, m := range reTSImportSide.FindAllStringSubmatch(content, -1) {
			out = append(out, m[1])
		}
	case ".py":
		for _, m := range rePyImport.FindAllStringSubmatch(content, -1) {
			if m[1] != "" {
				out = append(out, m[1])
			} else if m[2] != "" {
				out = append(out, m[2])
			}
		}
	case ".java", ".kt":
		for _, m := range reJavaImport.FindAllStringSubmatch(content, -1) {
			out = append(out, m[1])
		}
	default:
		// generic: from 'x' / import "x"
		for _, m := range reTSImportFrom.FindAllStringSubmatch(content, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

func resolveImport(from, imp string, nodes []string, baseIndex map[string][]string) []string {
	imp = strings.TrimSpace(imp)
	if imp == "" || strings.HasPrefix(imp, "http") {
		return nil
	}
	// Relative JS/TS
	if strings.HasPrefix(imp, ".") {
		dir := filepath.ToSlash(filepath.Dir(from))
		cand := filepath.ToSlash(filepath.Clean(dir + "/" + imp))
		var hits []string
		for _, n := range nodes {
			if n == cand || strings.TrimSuffix(n, filepath.Ext(n)) == cand {
				hits = append(hits, n)
			}
			// try with extensions
			for _, ext := range []string{".ts", ".tsx", ".js", ".jsx", ".go"} {
				if n == cand+ext {
					hits = append(hits, n)
				}
			}
		}
		return unique(hits)
	}
	// Go module path suffix match / basename
	base := imp
	if i := strings.LastIndex(imp, "/"); i >= 0 {
		base = imp[i+1:]
	}
	if i := strings.LastIndex(base, "."); i >= 0 && !strings.Contains(imp, "/") {
		// python module.sub → last segment
		base = base[i+1:]
	}
	// java package.Class
	if strings.Contains(imp, ".") && (strings.HasSuffix(from, ".java") || strings.HasSuffix(from, ".kt")) {
		parts := strings.Split(imp, ".")
		base = parts[len(parts)-1]
	}
	return unique(baseIndex[base])
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// DirKey returns the directory used for packaging clusters.
func DirKey(path string) string {
	d := filepath.ToSlash(filepath.Dir(path))
	if d == "." || d == "" {
		return "_root"
	}
	return d
}
