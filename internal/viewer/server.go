package viewer

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alibaba/open-code-review/internal/reviewstore"
)

//go:embed templates/*.html static/style.css
var assets embed.FS

// ServerOptions configures the viewer HTTP server.
type ServerOptions struct {
	Addr       string
	ReviewsDir string
}

func StartServer(addr string) error {
	return StartServerWithOptions(ServerOptions{Addr: addr})
}

func StartServerWithOptions(opts ServerOptions) error {
	root, err := SessionsRoot()
	if err != nil {
		return fmt.Errorf("resolve sessions root: %w", err)
	}

	reviewsRoot := opts.ReviewsDir
	if reviewsRoot == "" {
		reviewsRoot = os.Getenv("OCR_REVIEWS_DIR")
	}

	mux := http.NewServeMux()

	// Static assets (must be registered before "/" catch-all)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS()))))

	// Routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleRepos(w, r, root)
	})
	mux.HandleFunc("/r/{repo}", func(w http.ResponseWriter, r *http.Request) {
		repo := r.PathValue("repo")
		if !isValidPathSegment(repo) {
			http.Error(w, "invalid repo path", http.StatusBadRequest)
			return
		}
		handleSessions(w, r, root, repo)
	})
	mux.HandleFunc("/r/{repo}/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		repo := r.PathValue("repo")
		sid := r.PathValue("sessionID")
		if !isValidPathSegment(repo) || !isValidPathSegment(sid) {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}
		handleSession(w, r, root, repo, sid)
	})

	// JSON API for review results
	if reviewsRoot != "" {
		mux.HandleFunc("/api/reviews", func(w http.ResponseWriter, r *http.Request) {
			handleAPIReviews(w, r, reviewsRoot)
		})
		mux.HandleFunc("/api/reviews/{project}", func(w http.ResponseWriter, r *http.Request) {
			project := r.PathValue("project")
			if !isValidPathSegment(project) {
				writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid project"))
				return
			}
			handleAPIProjectReviews(w, r, reviewsRoot, project)
		})
		mux.HandleFunc("/api/reviews/{project}/{reviewID}", func(w http.ResponseWriter, r *http.Request) {
			project := r.PathValue("project")
			reviewID := r.PathValue("reviewID")
			if !isValidPathSegment(project) || !isValidPathSegment(reviewID) {
				writeJSONError(w, http.StatusBadRequest, fmt.Errorf("invalid path"))
				return
			}
			handleAPIReviewDetail(w, r, reviewsRoot, project, reviewID)
		})
	}

	// Wrap the mux with a Host-header allowlist. Without this, any web page
	// the user visits can DNS-rebind its origin to 127.0.0.1 and read the
	// session JSONL exposed by this viewer (which contains LLM request bodies
	// = source code being reviewed and the LLM's analysis of it).
	allowed := resolveAllowedHostsFromEnv(opts.Addr)
	guarded := hostGuard(allowed, mux)

	srv := &http.Server{
		Addr:    opts.Addr,
		Handler: guarded,
	}

	fmt.Printf("\nOpen browser: http://%s\n", opts.Addr)
	return srv.ListenAndServe()
}

var cstZone = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	return loc
}()

// isValidPathSegment delegates to reviewstore.IsSafePathSegment so the
// HTTP-layer validation stays consistent with the store-layer validation.
func isValidPathSegment(segment string) bool {
	return reviewstore.IsSafePathSegment(segment)
}

func formatTime(t time.Time) string {
	return t.In(cstZone).Format("2006-01-02 15:04")
}

func parseTemplate(name string) (*template.Template, error) {
	funcMap := template.FuncMap{
		"formatDuration": formatDuration,
		"formatTime":     formatTime,
		"truncate":       truncateText,
		"formatNumber":   formatNumber,
		"add":            func(a, b int) int { return a + b },
		"lower":          strings.ToLower,
		"cardCount": func(tasks map[TaskType][]*TaskCard) int {
			n := 0
			for _, cards := range tasks {
				n += len(cards)
			}
			return n
		},
		"taskTypeClass": func(tt TaskType) string {
			switch tt {
			case PlanTask:
				return "task-plan"
			case MainTask:
				return "task-main"
			case MemoryCompressionTask:
				return "task-memory"
			case ReLocationTask:
				return "task-relocation"
			default:
				return "task-default"
			}
		},
		"severityClass": func(s string) string {
			switch strings.ToLower(s) {
			case "critical":
				return "severity-critical"
			case "high":
				return "severity-high"
			case "medium":
				return "severity-medium"
			case "low":
				return "severity-low"
			default:
				return "severity-low"
			}
		},
		"categoryIcon": func(c string) string {
			switch strings.ToLower(c) {
			case "bug":
				return "\U0001F41B"
			case "security":
				return "\U0001F512"
			case "performance":
				return "⚡"
			case "maintainability":
				return "\U0001F527"
			case "test":
				return "✅"
			case "style":
				return "\U0001F3A8"
			case "documentation":
				return "\U0001F4DA"
			default:
				return "\U0001F4DD"
			}
		},
		"severityOrder": func() []string {
			return []string{"critical", "high", "medium", "low"}
		},
		"categoryOrder": func() []string {
			return []string{"bug", "security", "performance", "maintainability", "test", "style", "documentation"}
		},
		"orderedTasks": func(tasks map[TaskType][]*TaskCard) []struct {
			Type  TaskType
			Cards []*TaskCard
		} {
			order := []TaskType{PlanTask, MainTask, ReLocationTask, MemoryCompressionTask}
			var result []struct {
				Type  TaskType
				Cards []*TaskCard
			}
			for _, tt := range order {
				if cards, ok := tasks[tt]; ok {
					result = append(result, struct {
						Type  TaskType
						Cards []*TaskCard
					}{tt, cards})
				}
			}
			for tt, cards := range tasks {
				if tt != PlanTask && tt != MainTask && tt != ReLocationTask && tt != MemoryCompressionTask {
					result = append(result, struct {
						Type  TaskType
						Cards []*TaskCard
					}{tt, cards})
				}
			}
			return result
		},
	}
	content, err := assets.ReadFile("templates/" + name)
	if err != nil {
		return nil, err
	}
	return template.New(name).Funcs(funcMap).Parse(string(content))
}

func truncateText(n int, s string) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	tmpl, err := parseTemplate(name)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		// Partially written — just log
		fmt.Printf("[ocr] template execution error: %v\n", err)
	}
}

func staticFS() fs.FS {
	sub, err := fs.Sub(assets, "static")
	if err != nil {
		panic(err)
	}
	return sub
}

func formatNumber(n int) string {
	var display string
	switch {
	case n >= 1_000_000:
		if n%1_000_000 == 0 {
			display = fmt.Sprintf("%dM", n/1_000_000)
		} else {
			display = trimFloatSuffix(fmt.Sprintf("%.2fM", float64(n)/1_000_000))
		}
	case n >= 1_000:
		if n%1_000 == 0 {
			display = fmt.Sprintf("%dK", n/1_000)
		} else {
			display = trimFloatSuffix(fmt.Sprintf("%.2fK", float64(n)/1_000))
		}
	default:
		display = strconv.Itoa(n)
	}
	return display
}

// trimFloatSuffix removes trailing zeros and the trailing dot from a
// floating-point string like "1.10K" → "1.1K", "1.00K" → "1K".
func trimFloatSuffix(s string) string {
	// Find the dot position before the suffix (K/M).
	// Input is always "%d.%dX" or "%dX".
	dot := strings.LastIndexByte(s, '.')
	if dot < 0 {
		return s
	}
	// Find the suffix letter (K or M) — it's always the last character.
	suffix := s[len(s)-1]
	mantissa := s[:len(s)-1] // strip suffix

	// Trim trailing zeros from the fractional part.
	i := len(mantissa) - 1
	for i >= 0 && mantissa[i] == '0' {
		i--
	}
	if i >= 0 && mantissa[i] == '.' {
		i-- // also trim the dot if whole fractional part was zeros
	}
	return mantissa[:i+1] + string(suffix)
}

func formatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", seconds)
	}
	minutes := int(d.Minutes())
	sec := int(d.Seconds()) - minutes*60
	return fmt.Sprintf("%dm%ds", minutes, sec)
}
