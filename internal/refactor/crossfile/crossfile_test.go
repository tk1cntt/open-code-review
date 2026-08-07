package crossfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alibaba/open-code-review/internal/model"
)

func TestParseMode(t *testing.T) {
	if ParseMode("") != ModeLocal {
		t.Fatal("default local")
	}
	if ParseMode("CROSS") != ModeCross {
		t.Fatal("cross")
	}
	if ParseMode("full") != ModeFull {
		t.Fatal("full")
	}
}

func TestBuildTopologyAndSkeleton(t *testing.T) {
	files := []FileInput{
		{Path: "pkg/a/a.go", Content: "package a\n\nimport \"pkg/b\"\n\nfunc Foo() {}\n"},
		{Path: "pkg/b/b.go", Content: "package b\n\nfunc Bar() {}\n"},
		{Path: "pkg/a/c.go", Content: "package a\n\nfunc Baz() {}\n"},
	}
	topo := BuildTopology(files)
	if len(topo.Nodes) != 3 {
		t.Fatalf("nodes=%d", len(topo.Nodes))
	}
	// same_dir edge between a.go and c.go
	sameDir := false
	for _, e := range topo.Edges {
		if e.Kind == "same_dir" && ((e.From == "pkg/a/a.go" && e.To == "pkg/a/c.go") || (e.From == "pkg/a/c.go" && e.To == "pkg/a/a.go")) {
			sameDir = true
		}
	}
	if !sameDir {
		t.Fatal("expected same_dir edge")
	}
	sk := ExtractSkeleton("pkg/a/a.go", files[0].Content)
	if !strings.Contains(sk, "Foo") {
		t.Fatalf("skeleton missing Foo: %s", sk)
	}
}

func TestBuildClustersSimilarity(t *testing.T) {
	// Two files with nearly identical function bodies
	body := `
func ProcessPayment(amount int) error {
    if amount <= 0 {
        return ErrInvalid
    }
    if err := validateUser(); err != nil {
        return err
    }
    return chargeCard(amount)
}
`
	files := []FileInput{
		{Path: "svc/a.go", Content: "package svc\n" + body},
		{Path: "svc/b.go", Content: "package svc\n" + strings.Replace(body, "chargeCard", "chargeCard", 1)},
		{Path: "other/x.go", Content: "package other\n\nfunc Unrelated() { println(1) }\n"},
	}
	opts := DefaultClusterOptions()
	opts.UseSimilarity = true
	opts.MinSimilarity = 0.5
	clusters := BuildClusters(files, opts)
	if len(clusters) == 0 {
		t.Fatal("no clusters")
	}
	// a.go and b.go should be co-clustered
	found := false
	for _, c := range clusters {
		paths := map[string]bool{}
		for _, f := range c.Files {
			paths[f.Path] = true
		}
		if paths["svc/a.go"] && paths["svc/b.go"] {
			found = true
			if len(c.Skeletons) == 0 {
				t.Fatal("expected skeletons")
			}
		}
	}
	if !found {
		t.Fatalf("a.go and b.go not co-clustered: %+v", clusterPaths(clusters))
	}
}

func clusterPaths(cs []Cluster) [][]string {
	var out [][]string
	for _, c := range cs {
		var p []string
		for _, f := range c.Files {
			p = append(p, f.Path)
		}
		out = append(out, p)
	}
	return out
}

func TestJaccardIdentical(t *testing.T) {
	a := ShingleSet(NormalizeCode("func foo() { return 1 }"), 5)
	b := ShingleSet(NormalizeCode("func foo() { return 1 }"), 5)
	if Jaccard(a, b) < 0.99 {
		t.Fatalf("expected ~1, got %v", Jaccard(a, b))
	}
}

func TestParseSmellAndPlan(t *testing.T) {
	smells, err := ParseSmellReports(`[{"smell_type":"DUPLICATED_FLOW","affected_files":["a.go","b.go"],"evidence":[{"path":"a.go","start_line":1,"end_line":5}],"rule_id":"REF-XDUP-001","message":"dup"}]`)
	if err != nil || len(smells) != 1 {
		t.Fatalf("%v %v", err, smells)
	}
	cm := SmellsToComments(smells, "c1")
	if len(cm) != 1 || cm[0].Path != "a.go" || len(cm[0].RelatedLocations) == 0 {
		t.Fatalf("%+v", cm)
	}

	plans, err := ParseRefactorPlans(`{"plan_id":"p1","summary":"extract","steps":[{"order":2,"action":"rewrite_callsite","path":"b.go"},{"order":1,"action":"create_file","path":"h.go","suggestion_code":"package h\n"}]}`)
	if err != nil || len(plans) != 1 {
		t.Fatalf("%v", err)
	}
	pc := PlansToComments(plans, "c1")
	if len(pc) != 1 || pc[0].PlanID != "p1" {
		t.Fatalf("%+v", pc)
	}
}

func TestParseSmellReports_EmptyAndProse(t *testing.T) {
	// Empty / no-op cases.
	for _, raw := range []string{"", "[]", "null", "```json\n[]\n```"} {
		smells, err := ParseSmellReports(raw)
		if err != nil || len(smells) != 0 {
			t.Fatalf("empty %q: smells=%v err=%v", raw, smells, err)
		}
	}

	// Prose-wrapped JSON still parses.
	prose := "Here are the smells I found:\n[{\"smell_type\":\"DUPLICATED_FLOW\",\"affected_files\":[\"a.go\"],\"evidence\":[{\"path\":\"a.go\",\"start_line\":1,\"end_line\":2}],\"message\":\"dup\"}]\nThanks."
	smells, err := ParseSmellReports(prose)
	if err != nil || len(smells) != 1 {
		t.Fatalf("prose wrap: %v %v", err, smells)
	}

	// Non-empty garbage must error, not silently succeed.
	if _, err := ParseSmellReports("I looked carefully but found nothing structured."); err == nil {
		t.Fatal("expected error for non-JSON prose")
	}
}

func TestParseRefactorPlans_EmptyAndProse(t *testing.T) {
	for _, raw := range []string{"", "[]", "{}", "null"} {
		plans, err := ParseRefactorPlans(raw)
		if err != nil || len(plans) != 0 {
			t.Fatalf("empty %q: plans=%v err=%v", raw, plans, err)
		}
	}

	prose := "Plan below:\n{\"plan_id\":\"p9\",\"summary\":\"extract helper\",\"steps\":[{\"order\":1,\"action\":\"create_file\",\"path\":\"h.go\"}]}\n"
	plans, err := ParseRefactorPlans(prose)
	if err != nil || len(plans) != 1 || plans[0].PlanID != "p9" {
		t.Fatalf("prose wrap: %v %+v", err, plans)
	}

	if _, err := ParseRefactorPlans("no json here at all"); err == nil {
		t.Fatal("expected error for non-JSON prose")
	}
}

func TestExtractJSON_StringAware(t *testing.T) {
	// Braces inside strings must not end the object early.
	raw := `note: {"msg":"use {curly} braces","n":1} trailing`
	got := extractJSON(raw)
	want := `{"msg":"use {curly} braces","n":1}`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	// Mismatched outer pair should not be treated as clean JSON.
	if extractJSON(`{not closed`) != "" {
		t.Fatal("unbalanced should return empty")
	}
}

func TestRenderClusterPrompt(t *testing.T) {
	files := []FileInput{
		{Path: "a.go", Content: "package a\nfunc A(){}\n"},
		{Path: "b.go", Content: "package a\nfunc B(){}\n"},
	}
	c := BuildClusters(files, DefaultClusterOptions())[0]
	s := RenderClusterPrompt(c)
	if !strings.Contains(s, "project_graph") || !strings.Contains(s, "file_skeletons") {
		t.Fatal(s)
	}
}

func TestVerifyAndApplyRollback(t *testing.T) {
	dir := t.TempDir()
	// valid go file
	p := filepath.Join(dir, "ok.go")
	if err := os.WriteFile(p, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v := VerifyFiles(dir, []string{"ok.go"}, false)
	if !v.OK {
		t.Fatalf("%v", v.Messages)
	}

	// apply bad plan then rollback
	plans := []RefactorPlan{{
		PlanID: "bad",
		Steps: []PlanStep{{
			Order: 1, Action: "create_file", Path: "bad.go",
			SuggestionCode: "package main\nfunc main( {\n", // syntax error
		}},
	}}
	res := ApplyPlanSteps(dir, plans, false)
	if res.Verify.OK {
		t.Fatal("expected verify fail")
	}
	if !res.RolledBack {
		t.Fatal("expected rollback")
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.go")); !os.IsNotExist(err) {
		t.Fatal("bad.go should be removed")
	}

	// good apply
	plans2 := []RefactorPlan{{
		PlanID: "good",
		Steps: []PlanStep{{
			Order: 1, Action: "create_file", Path: "good.go",
			SuggestionCode: "package main\n\nfunc Helper() int { return 1 }\n",
		}},
	}}
	res2 := ApplyPlanSteps(dir, plans2, false)
	if !res2.Verify.OK {
		t.Fatalf("%v", res2.Messages)
	}
	if _, err := os.Stat(filepath.Join(dir, "good.go")); err != nil {
		t.Fatal(err)
	}
}

func TestCrossFileRulesEmbeddedPathConvention(t *testing.T) {
	// document expected rule path for agent wiring
	const want = "refactoring/cross_file/common.md"
	if want == "" {
		t.Fatal("empty")
	}
}

func TestApplyComments_BasicLineReplacement(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n\nvar message = \"hello\"\n\nfunc main() {\n\tprintln(message)\n}\n"
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	comments := []model.LlmComment{{
		Path:           "main.go",
		StartLine:      3,
		EndLine:        3,
		SuggestionCode: "var message = \"hello, world\"",
	}}
	res := ApplyComments(dir, comments, false)
	if !res.Verify.OK || len(res.Written) != 1 {
		t.Fatalf("expected 1 written, verify ok; got written=%v verify=%v msgs=%v", res.Written, res.Verify.OK, res.Messages)
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "hello, world") {
		t.Fatalf("file not modified: %s", string(data))
	}
}

func TestApplyComments_RollbackOnInvalidGo(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n\nfunc main() {\n\tx := 1\n}\n"
	p := filepath.Join(dir, "main.go")
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	comments := []model.LlmComment{{
		Path:           "main.go",
		StartLine:      3,
		EndLine:        3,
		SuggestionCode: "func main( {", // syntax error
	}}
	res := ApplyComments(dir, comments, false)
	if res.Verify.OK {
		t.Fatal("expected verify failure for invalid Go")
	}
	if !res.RolledBack {
		t.Fatal("expected rollback")
	}
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "x := 1") {
		t.Fatalf("rollback failed, file is: %s", string(data))
	}
}

func TestApplyComments_SkipsEmptySuggestion(t *testing.T) {
	dir := t.TempDir()
	comments := []model.LlmComment{
		{Path: "main.go", StartLine: 1, EndLine: 1, SuggestionCode: ""},
		{Path: "", StartLine: 1, EndLine: 1, SuggestionCode: "x"},
		{Path: "main.go", StartLine: 0, EndLine: 0, SuggestionCode: "x"},
	}
	res := ApplyComments(dir, comments, false)
	if len(res.Skipped) != 3 || len(res.Written) != 0 {
		t.Fatalf("expected all skipped: written=%v skipped=%v", res.Written, res.Skipped)
	}
}

func TestApplyComments_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main\nfunc a() { return }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte("package main\nfunc b() { return }\n"), 0o644)

	comments := []model.LlmComment{
		{Path: "a.go", StartLine: 2, EndLine: 2, SuggestionCode: "func a() int { return 1 }"},
		{Path: "b.go", StartLine: 2, EndLine: 2, SuggestionCode: "func b() int { return 2 }"},
	}
	res := ApplyComments(dir, comments, false)
	if !res.Verify.OK || len(res.Written) != 2 {
		t.Fatalf("expected 2 written; written=%v ok=%v msgs=%v", res.Written, res.Verify.OK, res.Messages)
	}
}

func TestApplyComments_OverlappingRanges(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n\nvar x int\nvar y int\nvar z int\n"
	p := filepath.Join(dir, "main.go")
	os.WriteFile(p, []byte(src), 0o644)

	// Two comments with overlapping line ranges on the same file.
	comments := []model.LlmComment{
		{Path: "main.go", StartLine: 3, EndLine: 4, SuggestionCode: "var a int"},
		{Path: "main.go", StartLine: 4, EndLine: 5, SuggestionCode: "var b int"},
	}
	res := ApplyComments(dir, comments, false)
	if len(res.Written) != 0 {
		t.Fatal("expected overlapping range comments to be skipped")
	}
	found := false
	for _, s := range res.Skipped {
		if strings.Contains(s, "overlapping") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected overlapping skip message in: %v", res.Skipped)
	}
}

func TestApplyComments_EndLineNormalized(t *testing.T) {
	dir := t.TempDir()
	src := "package main\n\nvar a = 1\nvar b = 2\nvar c = 3\n"
	p := filepath.Join(dir, "main.go")
	os.WriteFile(p, []byte(src), 0o644)

	comments := []model.LlmComment{
		{Path: "main.go", StartLine: 3, EndLine: 0, SuggestionCode: "var a = 10"},
	}
	res := ApplyComments(dir, comments, false)
	if !res.Verify.OK || len(res.Written) != 1 {
		t.Fatalf("expected 1 written, verify ok; got written=%v ok=%v msgs=%v", res.Written, res.Verify.OK, res.Messages)
	}
	data, _ := os.ReadFile(p)
	if strings.Count(string(data), "var a") != 1 {
		t.Fatalf("expected exactly 1 'var a' (no duplication); got: %s", string(data))
	}
	if !strings.Contains(string(data), "var b = 2") {
		t.Fatal("expected var b to be preserved")
	}
}
