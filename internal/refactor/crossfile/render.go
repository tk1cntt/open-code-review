package crossfile

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RenderClusterPrompt builds the structured context block for X1/X2 prompts.
func RenderClusterPrompt(c Cluster) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<cluster_id>%s</cluster_id>\n", c.ID)
	fmt.Fprintf(&b, "<file_count>%d</file_count>\n\n", len(c.Files))

	if c.Graph != nil {
		// Compact JSON graph (import edges only to reduce noise)
		type g struct {
			Nodes []string       `json:"nodes"`
			Edges []TopologyEdge `json:"edges"`
		}
		var importEdges []TopologyEdge
		for _, e := range c.Graph.Edges {
			if e.Kind == "import" {
				importEdges = append(importEdges, e)
			}
		}
		payload, err := json.Marshal(g{Nodes: c.Graph.Nodes, Edges: importEdges})
		if err == nil {
			fmt.Fprintf(&b, "<project_graph>\n%s\n</project_graph>\n\n", string(payload))
		}
	}

	b.WriteString("<file_skeletons>\n")
	for _, f := range c.Files {
		sk := c.Skeletons[f.Path]
		if sk == "" {
			sk = ExtractSkeleton(f.Path, f.Content)
		}
		fmt.Fprintf(&b, "### %s\n```\n%s\n```\n", f.Path, sk)
	}
	b.WriteString("</file_skeletons>\n\n")

	if len(c.Slices) > 0 {
		b.WriteString("<code_slices>\n")
		for _, s := range c.Slices {
			fmt.Fprintf(&b, "<slice path=%q lines=%q score=\"%.2f\">\n```\n%s\n```\n</slice>\n",
				s.Path, fmt.Sprintf("%d-%d", s.StartLine, s.EndLine), s.Score, s.Content)
		}
		b.WriteString("</code_slices>\n\n")
	} else if len(c.Files) <= 4 {
		// Small cluster: include truncated full files
		b.WriteString("<files>\n")
		for _, f := range c.Files {
			body := f.Content
			if len(body) > 12_000 {
				// Walk back from cutoff to avoid splitting a UTF-8 character.
				i := 12_000
				for i > 0 && i < len(body) && body[i]&0xC0 == 0x80 {
					i--
				}
				body = body[:i] + "\n…[truncated]"
			}
			fmt.Fprintf(&b, "<file path=%q>\n```\n%s\n```\n</file>\n", f.Path, body)
		}
		b.WriteString("</files>\n")
	}
	return b.String()
}

// RenderFilesForArchitect includes full content of affected paths only.
func RenderFilesForArchitect(c Cluster, affected []string) string {
	want := map[string]struct{}{}
	for _, p := range affected {
		want[p] = struct{}{}
	}
	if len(want) == 0 {
		for _, f := range c.Files {
			want[f.Path] = struct{}{}
		}
	}
	var b strings.Builder
	b.WriteString("<files>\n")
	n := 0
	for _, f := range c.Files {
		if _, ok := want[f.Path]; !ok {
			continue
		}
		body := f.Content
		if len(body) > 20_000 {
			// Walk back from cutoff to avoid splitting a UTF-8 character.
			i := 20_000
			for i > 0 && i < len(body) && body[i]&0xC0 == 0x80 {
				i--
			}
			body = body[:i] + "\n…[truncated]"
		}
		fmt.Fprintf(&b, "<file path=%q>\n```\n%s\n```\n</file>\n", f.Path, body)
		n++
		if n >= 6 {
			break
		}
	}
	b.WriteString("</files>\n")
	return b.String()
}
