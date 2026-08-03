package crossfile

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// BuildClusters packs files into budgeted clusters using dir + import + optional similarity.
func BuildClusters(files []FileInput, opts ClusterOptions) []Cluster {
	if len(files) == 0 {
		return nil
	}
	if opts.MaxFilesPerCluster <= 0 {
		opts.MaxFilesPerCluster = 8
	}

	// Normalize paths
	norm := make([]FileInput, len(files))
	byPath := make(map[string]FileInput, len(files))
	for i, f := range files {
		p := filepath.ToSlash(f.Path)
		norm[i] = FileInput{Path: p, Content: f.Content}
		byPath[p] = norm[i]
	}

	topo := BuildTopology(norm)

	// Union-find seeded by same_dir and import edges within set.
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if parent[x] == "" {
			parent[x] = x
		}
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[rb] = ra
		}
	}
	for _, f := range norm {
		_ = find(f.Path)
	}
	for _, e := range topo.Edges {
		if e.Kind == "import" || e.Kind == "same_dir" {
			if _, ok := byPath[e.To]; ok {
				union(e.From, e.To)
			}
		}
	}

	// F2: similarity edges
	var allWindows []FunctionWindow
	if opts.UseSimilarity {
		for _, f := range norm {
			allWindows = append(allWindows, ExtractWindows(f.Path, f.Content)...)
		}
		minSim := opts.MinSimilarity
		if minSim <= 0 {
			minSim = 0.78
		}
		for _, pair := range SimilarPairs(allWindows, minSim) {
			union(pair.A.Path, pair.B.Path)
		}
	}

	groups := map[string][]FileInput{}
	for _, f := range norm {
		r := find(f.Path)
		groups[r] = append(groups[r], f)
	}

	// Sort group keys for determinism
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var clusters []Cluster
	for _, k := range keys {
		members := groups[k]
		sort.Slice(members, func(i, j int) bool { return members[i].Path < members[j].Path })
		// Chunk by max files
		for start := 0; start < len(members); start += opts.MaxFilesPerCluster {
			end := start + opts.MaxFilesPerCluster
			if end > len(members) {
				end = len(members)
			}
			chunk := members[start:end]
			c := makeCluster(chunk, opts, allWindows)
			clusters = append(clusters, c)
		}
	}
	return clusters
}

func makeCluster(files []FileInput, opts ClusterOptions, allWindows []FunctionWindow) Cluster {
	id := "cluster"
	if len(files) > 0 {
		id = "cluster:" + DirKey(files[0].Path)
		if len(files) > 1 {
			id = fmt.Sprintf("cluster:%s+%d", DirKey(files[0].Path), len(files))
		}
	}
	topo := BuildTopology(files)
	skels := map[string]string{}
	skelBytes := 0
	for _, f := range files {
		sk := ExtractSkeleton(f.Path, f.Content)
		if opts.MaxSkeletonBytes > 0 && skelBytes+len(sk) > opts.MaxSkeletonBytes {
			// truncate skeleton list
			break
		}
		skels[f.Path] = sk
		skelBytes += len(sk)
	}

	// Slices from similar pairs within cluster
	pathSet := map[string]struct{}{}
	for _, f := range files {
		pathSet[f.Path] = struct{}{}
	}
	var windows []FunctionWindow
	for _, w := range allWindows {
		if _, ok := pathSet[w.Path]; ok {
			windows = append(windows, w)
		}
	}
	// If windows empty, extract now
	if len(windows) == 0 {
		for _, f := range files {
			windows = append(windows, ExtractWindows(f.Path, f.Content)...)
		}
	}
	minSim := opts.MinSimilarity
	if minSim <= 0 {
		minSim = 0.78
	}
	var slices []CodeSlice
	sliceBytes := 0
	seen := map[string]struct{}{}
	for _, pair := range SimilarPairs(windows, minSim) {
		for _, w := range []FunctionWindow{pair.A, pair.B} {
			key := fmt.Sprintf("%s:%d:%d", w.Path, w.StartLine, w.EndLine)
			if _, ok := seen[key]; ok {
				continue
			}
			// recover content lines
			content := windowContent(files, w)
			if content == "" {
				continue
			}
			if opts.MaxSliceBytes > 0 && sliceBytes+len(content) > opts.MaxSliceBytes {
				continue
			}
			seen[key] = struct{}{}
			slices = append(slices, CodeSlice{
				Path: w.Path, StartLine: w.StartLine, EndLine: w.EndLine,
				Content: content, Score: pair.Score,
			})
			sliceBytes += len(content)
		}
	}

	return Cluster{
		ID:        id,
		Files:     files,
		Graph:     topo,
		Skeletons: skels,
		Slices:    slices,
	}
}

func windowContent(files []FileInput, w FunctionWindow) string {
	for _, f := range files {
		if f.Path != w.Path {
			continue
		}
		lines := strings.Split(f.Content, "\n")
		if w.StartLine < 1 {
			w.StartLine = 1
		}
		if w.EndLine > len(lines) {
			w.EndLine = len(lines)
		}
		if w.StartLine > w.EndLine {
			return ""
		}
		return strings.Join(lines[w.StartLine-1:w.EndLine], "\n")
	}
	return ""
}
