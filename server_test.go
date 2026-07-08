package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/christophberger/quarto-sorter/internal/project"
)

// Keep test files out of the real system trash.
func TestMain(m *testing.M) {
	project.TrashCommands = nil
	os.Exit(m.Run())
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"index.qmd":           "---\ntitle: Home\norder: 1\n---\n# Home\n",
		"chapter2/index.qmd":  "---\ntitle: Chapter 2\norder: 2\n---\n# Two\n",
		"chapter2/second.qmd": "---\ntitle: Second\norder: 1\n---\n# Second\n",
		"chapter2/third.qmd":  "---\ntitle: Third\norder: 2\n---\n# Third\n",
		"chapter2/loose.qmd":  "---\ntitle: Loose\n---\n# Loose\n",
		"chapter2/broken.qmd": "---\ntitle: Broken\norder: 3\n---\n::: {.callout-note}\nunclosed\n",
		"_quarto.yml":         "project:\n  type: book\nbook:\n  chapters:\n    - index.qmd\n",
		"_quarto-print.yml":   "book:\n  chapters:\n    - index.qmd\n",
	}
	for name, content := range files {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func testServer(t *testing.T) (http.Handler, string) {
	t.Helper()
	root := fixture(t)
	srv, err := newServer()
	if err != nil {
		t.Fatal(err)
	}
	rec := post(t, srv, "/open", url.Values{"path": {root}})
	if rec.Code != http.StatusOK {
		t.Fatalf("open: status %d: %s", rec.Code, rec.Body)
	}
	return srv, root
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
	return rec
}

func post(t *testing.T, h http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestOpenRendersTree(t *testing.T) {
	srv, _ := testServer(t)
	body := get(t, srv, "/").Body.String()
	for _, want := range []string{
		`data-path="chapter2/second.qmd"`,
		`data-parent="chapter2/index.qmd"`,
		"Second",
		`value="print"`, // profile checkbox
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
	if !strings.Contains(body, `unordered" data-path="chapter2/loose.qmd"`) {
		t.Errorf("loose.qmd not marked unordered:\n%s", body)
	}
	if !strings.Contains(body, `bad-fences" data-path="chapter2/broken.qmd"`) {
		t.Errorf("broken.qmd not marked bad-fences:\n%s", body)
	}
	if strings.Contains(body, `bad-fences" data-path="chapter2/second.qmd"`) {
		t.Errorf("second.qmd wrongly marked bad-fences:\n%s", body)
	}
}

func TestMoveUpdatesFilesAndYaml(t *testing.T) {
	srv, root := testServer(t)
	rec := post(t, srv, "/move", url.Values{
		"src": {"chapter2/third.qmd"}, "parent": {"chapter2/index.qmd"}, "pos": {"0"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("move: status %d: %s", rec.Code, rec.Body)
	}
	body := rec.Body.String()
	if strings.Index(body, "chapter2/third.qmd") > strings.Index(body, "chapter2/second.qmd") {
		t.Errorf("third not before second:\n%s", body)
	}
	for _, cfg := range []string{"_quarto.yml", "_quarto-print.yml"} {
		yml, _ := os.ReadFile(filepath.Join(root, cfg))
		s := string(yml)
		if !strings.Contains(s, "- chapter2/third.qmd") {
			t.Errorf("%s missing chapters: %s", cfg, s)
		}
		if strings.Index(s, "- chapter2/third.qmd") > strings.Index(s, "- chapter2/second.qmd") {
			t.Errorf("%s chapter order wrong: %s", cfg, s)
		}
	}
}

func TestProfileSelectionLimitsYamlWrites(t *testing.T) {
	srv, root := testServer(t)
	// Deselect all profiles, then move: only _quarto.yml gets rewritten.
	if rec := post(t, srv, "/profiles", url.Values{}); rec.Code != http.StatusNoContent {
		t.Fatalf("profiles: status %d", rec.Code)
	}
	post(t, srv, "/move", url.Values{
		"src": {"chapter2/third.qmd"}, "parent": {"chapter2/index.qmd"}, "pos": {"0"},
	})
	yml, _ := os.ReadFile(filepath.Join(root, "_quarto-print.yml"))
	if strings.Contains(string(yml), "third.qmd") {
		t.Errorf("_quarto-print.yml written although deselected: %s", yml)
	}
}

func TestContent(t *testing.T) {
	srv, _ := testServer(t)
	body := get(t, srv, "/content?path=chapter2/second.qmd").Body.String()
	if !strings.Contains(body, "# Second") {
		t.Errorf("content missing file body:\n%s", body)
	}
	if rec := get(t, srv, "/content?path=../outside.qmd"); rec.Code != http.StatusBadRequest {
		t.Errorf("traversal: status %d, want 400", rec.Code)
	}
	// A plain content fetch must not refresh the tree, a reload must.
	if strings.Contains(body, `hx-swap-oob="innerHTML:#tree"`) {
		t.Errorf("content without reload refreshes tree:\n%s", body)
	}
	reload := get(t, srv, "/content?path=chapter2/second.qmd&reload=1").Body.String()
	if !strings.Contains(reload, `hx-swap-oob="innerHTML:#tree"`) {
		t.Errorf("reload missing tree refresh:\n%s", reload)
	}
}

func TestSave(t *testing.T) {
	srv, root := testServer(t)
	newBody := "---\ntitle: Second\norder: 1\n---\n# Second updated\n"
	rec := post(t, srv, "/save", url.Values{"path": {"chapter2/second.qmd"}, "body": {newBody}})
	if rec.Code != http.StatusOK {
		t.Fatalf("save: status %d: %s", rec.Code, rec.Body)
	}
	// Save leaves the editor alone and updates the heading out of band;
	// the title is unchanged here, so the tree must not be refreshed.
	if !strings.Contains(rec.Body.String(), `id="content-title" hx-swap-oob="true"`) {
		t.Errorf("response missing heading update:\n%s", rec.Body)
	}
	if strings.Contains(rec.Body.String(), `hx-swap-oob="innerHTML:#tree"`) {
		t.Errorf("tree refreshed although title unchanged:\n%s", rec.Body)
	}
	got, err := os.ReadFile(filepath.Join(root, "chapter2/second.qmd"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != newBody {
		t.Errorf("file on disk not updated: %s", got)
	}

	// Changing the title must refresh the tree out of band.
	renamed := "---\ntitle: Second renamed\norder: 1\n---\n# Second updated\n"
	rec = post(t, srv, "/save", url.Values{"path": {"chapter2/second.qmd"}, "body": {renamed}})
	if rec.Code != http.StatusOK {
		t.Fatalf("save rename: status %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `hx-swap-oob="innerHTML:#tree"`) {
		t.Errorf("tree not refreshed after title change:\n%s", rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "Second renamed") {
		t.Errorf("response missing new title:\n%s", rec.Body)
	}

	if rec := post(t, srv, "/save", url.Values{"path": {"../outside.qmd"}, "body": {"x"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("traversal: status %d, want 400", rec.Code)
	}

	if rec := post(t, srv, "/save", url.Values{"path": {"nope.qmd"}, "body": {"x"}}); rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
		t.Errorf("nonexistent file: status %d, want 400 or 404", rec.Code)
	}
}

func TestCreateAndDelete(t *testing.T) {
	srv, root := testServer(t)
	rec := post(t, srv, "/create", url.Values{
		"parent": {""}, "name": {"about"}, "title": {"About"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: status %d: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `data-path="about.qmd"`) {
		t.Errorf("tree missing created page:\n%s", rec.Body)
	}
	yml, _ := os.ReadFile(filepath.Join(root, "_quarto.yml"))
	if !strings.Contains(string(yml), "- about.qmd") {
		t.Errorf("_quarto.yml missing created page: %s", yml)
	}

	rec = post(t, srv, "/delete", url.Values{"path": {"about.qmd"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: status %d: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), `data-path="about.qmd"`) {
		t.Errorf("tree still shows deleted page:\n%s", rec.Body)
	}
	if _, err := os.Stat(filepath.Join(root, "about.qmd")); !os.IsNotExist(err) {
		t.Error("about.qmd still on disk")
	}
}

func TestOpenBadPath(t *testing.T) {
	srv, err := newServer()
	if err != nil {
		t.Fatal(err)
	}
	if rec := post(t, srv, "/open", url.Values{"path": {"/no/such/dir"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("open bad path: status %d, want 400", rec.Code)
	}
}
