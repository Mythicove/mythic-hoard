package hoard

import (
	"bufio"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mythicove/mythic-hoard/internal/storage"
	"github.com/go-chi/chi/v5"
)

func specFile(t *testing.T) string {
	t.Helper()
	if env := os.Getenv("MYTHICOVE_API_SPECS"); env != "" {
		p := filepath.Join(env, "hoard.openapi.yaml")
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("MYTHICOVE_API_SPECS=%s set but spec file missing: %v", env, err)
		}
		return p
	}
	for _, dir := range []string{"../../../api/specs", "../../../../api/specs"} {
		p := filepath.Join(dir, "hoard.openapi.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("hoard.openapi.yaml not found; set MYTHICOVE_API_SPECS to enable drift gate")
	return ""
}

func parseSpecPaths(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	methods := map[string]bool{
		"get": true, "post": true, "put": true, "patch": true,
		"delete": true, "head": true, "options": true,
	}
	ops := map[string]bool{}
	sc := bufio.NewScanner(f)
	inPaths := false
	curPath := ""
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimRight(line, " \t")
		if trim == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		content := strings.TrimSpace(line)
		if indent == 0 {
			inPaths = trim == "paths:"
			curPath = ""
			continue
		}
		if !inPaths {
			continue
		}
		if indent == 2 && strings.HasPrefix(content, "/") && strings.HasSuffix(content, ":") {
			curPath = strings.TrimSuffix(content, ":")
			continue
		}
		if indent == 4 && curPath != "" {
			key := strings.TrimSuffix(content, ":")
			if methods[key] {
				ops[strings.ToUpper(key)+" "+curPath] = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(ops) == 0 {
		t.Fatalf("parsed 0 operations from %s", path)
	}
	return ops
}

func routerOps(t *testing.T) map[string]bool {
	t.Helper()
	files, err := storage.NewLocal(t.TempDir(), "http://localhost:8080")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(NewMemoryStore(), files, "http://localhost:8080")
	r := chi.NewRouter()
	// 与 cmd/server/main.go 一致：healthz/metrics 在 Register 之外单独注册。
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	svc.Register(r, "")

	ops := map[string]bool{}
	_ = chi.Walk(r, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		ops[method+" "+route] = true
		return nil
	})
	return ops
}

func TestSpecMatchesRoutes(t *testing.T) {
	spec := parseSpecPaths(t, specFile(t))
	routes := routerOps(t)
	var missing []string
	for op := range spec {
		if !routes[op] {
			missing = append(missing, op)
		}
	}
	if len(missing) > 0 {
		t.Errorf("spec documents operations not present in router:\n  %s", strings.Join(missing, "\n  "))
	}
}

func TestRoutesMatchSpec(t *testing.T) {
	spec := parseSpecPaths(t, specFile(t))
	routes := routerOps(t)
	allowOps := map[string]bool{"GET /metrics": true}
	allowPrefixes := []string{"/cdn/"}
	var extra []string
	for op := range routes {
		if allowOps[op] {
			continue
		}
		_, path, _ := strings.Cut(op, " ")
		allowed := false
		for _, p := range allowPrefixes {
			if strings.HasPrefix(path, p) {
				allowed = true
				break
			}
		}
		if allowed || spec[op] {
			continue
		}
		extra = append(extra, op)
	}
	if len(extra) > 0 {
		t.Errorf("router registers operations not documented in spec:\n  %s", strings.Join(extra, "\n  "))
	}
}
