package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeMultiFixture creates a multiproject root: a website root (no book
// chapters) with two book subprojects, one of which nests a section and
// carries _FW/_POL marked pages and flavor profiles.
//
//	index.qmd                  (order 1, root page)
//	suba/index.qmd              (order 2 at root, book subproject)
//	suba/care/index.qmd         (order 1 in suba, nested section)
//	suba/care/water.qmd         (order 1 in care)
//	suba/x.qmd                  (order 2 in suba)
//	suba/truck_FW.qmd           (order 3 in suba, 🚒 marked)
//	suba/dispatch_POL.qmd       (order 4 in suba, 🚔 marked)
//	subb/index.qmd               (order 3 at root, book subproject)
//	subc/                        (subproject without a book key)
//	assets/notes.txt             (not a subproject: no _quarto.yml)
//	_drafts/_quarto.yml          (skipped: underscore-prefixed folder)
func writeMultiFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	pages := map[string]string{
		"index.qmd":             "---\ntitle: Home\norder: 1\n---\n# Home\n",
		"suba/index.qmd":        "---\ntitle: Suba\norder: 2\n---\n# Suba\n",
		"suba/care/index.qmd":   "---\ntitle: Care\norder: 1\n---\n# Care\n",
		"suba/care/water.qmd":   "---\ntitle: Water\norder: 1\n---\n# Water\n",
		"suba/x.qmd":            "---\ntitle: X\norder: 2\n---\n# X\n",
		"suba/truck_FW.qmd":     "---\ntitle: Truck\norder: 3\n---\n# Truck\n",
		"suba/dispatch_POL.qmd": "---\ntitle: Dispatch\norder: 4\n---\n# Dispatch\n",
		"subb/index.qmd":        "---\ntitle: Subb\norder: 3\n---\n# Subb\n",
	}
	other := map[string]string{
		"_quarto.yml":              "project:\n  type: website\n",
		"suba/_quarto.yml":         "project:\n  type: book\nbook:\n  title: Suba Book # keep\n  chapters:\n    - index.qmd\n    - stale.qmd\n",
		"suba/_quarto-fw.yml":      "make:\n  handout:\n  slides:\n",
		"suba/_quarto-handout.yml": "make:\n  handout:\n  slides:\n",
		"suba/_quarto-notes.yml":   "format:\n  pdf: default\n",
		"subb/_quarto.yml":         "project:\n  type: book\nbook:\n  title: Subb Book\n  chapters:\n    - index.qmd\n",
		"subc/_quarto.yml":         "project:\n  type: website\n",
		"_drafts/_quarto.yml":      "project:\n  type: book\nbook:\n  title: Draft\n",
		"assets/notes.txt":         "not a project\n",
	}
	for name, content := range pages {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, content := range other {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// Subprojects lists immediate subfolders with a _quarto.yml, skipping
// folders without one and underscore-prefixed folders even if they have one.
func TestSubprojects(t *testing.T) {
	root := writeMultiFixture(t)
	got, err := Subprojects(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"suba", "subb", "subc"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Subprojects() = %v, want %v", got, want)
	}
}

// In multiproject mode, Profiles lists <sub>/<name> for subproject
// _quarto-<name>.yml files that have a top-level make key.
func TestProfilesMultiproject(t *testing.T) {
	root := writeMultiFixture(t)
	got, err := Profiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"suba/fw", "suba/handout"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Profiles() = %v, want %v", got, want)
	}
}

func TestFlavorOf(t *testing.T) {
	tests := []struct {
		name string
		want map[string]bool
	}{
		{"handout", map[string]bool{}},
		{"fw", map[string]bool{markerFW: true}},
		{"handout-fw", map[string]bool{markerFW: true}},
		{"fw-pol", map[string]bool{markerFW: true, markerPOL: true}},
	}
	for _, tt := range tests {
		if got := flavorOf(tt.name); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("flavorOf(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// WriteChapters updates every book subproject's _quarto.yml with its own
// pages, in paths relative to the subfolder, regardless of which flavor
// profiles are selected. The website root (no book.chapters) and a
// subproject without a book key stay untouched. An unselected profile
// stays untouched too.
func TestWriteChaptersMultiproject(t *testing.T) {
	root := writeMultiFixture(t)
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}

	if err := tree.WriteChapters(nil); err != nil {
		t.Fatal(err)
	}

	suba, _ := os.ReadFile(filepath.Join(root, "suba", "_quarto.yml"))
	for _, want := range []string{
		"- index.qmd", "- care/index.qmd", "- care/water.qmd",
		"- x.qmd", "- truck_FW.qmd", "- dispatch_POL.qmd",
		"title: Suba Book # keep",
	} {
		if !strings.Contains(string(suba), want) {
			t.Errorf("suba/_quarto.yml missing %q:\n%s", want, suba)
		}
	}
	for _, stray := range []string{"stale.qmd", "suba/index.qmd", "suba/care"} {
		if strings.Contains(string(suba), stray) {
			t.Errorf("suba/_quarto.yml contains %q, want subfolder-relative paths:\n%s", stray, suba)
		}
	}

	subb, _ := os.ReadFile(filepath.Join(root, "subb", "_quarto.yml"))
	if !strings.Contains(string(subb), "- index.qmd") {
		t.Errorf("subb/_quarto.yml not updated although unselected:\n%s", subb)
	}

	rootYml, _ := os.ReadFile(filepath.Join(root, "_quarto.yml"))
	if string(rootYml) != "project:\n  type: website\n" {
		t.Errorf("root _quarto.yml changed:\n%s", rootYml)
	}

	subc, _ := os.ReadFile(filepath.Join(root, "subc", "_quarto.yml"))
	if string(subc) != "project:\n  type: website\n" {
		t.Errorf("subc/_quarto.yml (no book key) changed:\n%s", subc)
	}

	fw, _ := os.ReadFile(filepath.Join(root, "suba", "_quarto-fw.yml"))
	if string(fw) != "make:\n  handout:\n  slides:\n" {
		t.Errorf("suba/_quarto-fw.yml changed although unselected:\n%s", fw)
	}
	handout, _ := os.ReadFile(filepath.Join(root, "suba", "_quarto-handout.yml"))
	if string(handout) != "make:\n  handout:\n  slides:\n" {
		t.Errorf("suba/_quarto-handout.yml changed although unselected:\n%s", handout)
	}

	// Selecting handout updates it while fw, still unselected, stays put.
	if err := tree.WriteChapters([]string{"suba/handout"}); err != nil {
		t.Fatal(err)
	}
	handout, _ = os.ReadFile(filepath.Join(root, "suba", "_quarto-handout.yml"))
	if !strings.Contains(string(handout), "- x.qmd") {
		t.Errorf("suba/_quarto-handout.yml not updated after selection:\n%s", handout)
	}
	fw, _ = os.ReadFile(filepath.Join(root, "suba", "_quarto-fw.yml"))
	if string(fw) != "make:\n  handout:\n  slides:\n" {
		t.Errorf("suba/_quarto-fw.yml changed although still unselected:\n%s", fw)
	}
}

// A flavor profile drops pages marked for a different flavor; unmarked
// pages and a profile with no flavor token (handout) keep every page.
func TestWriteChaptersMultiprojectFlavor(t *testing.T) {
	root := writeMultiFixture(t)
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.WriteChapters([]string{"suba/fw", "suba/handout"}); err != nil {
		t.Fatal(err)
	}

	fw, _ := os.ReadFile(filepath.Join(root, "suba", "_quarto-fw.yml"))
	for _, want := range []string{"- index.qmd", "- care/index.qmd", "- care/water.qmd", "- x.qmd", "- truck_FW.qmd"} {
		if !strings.Contains(string(fw), want) {
			t.Errorf("suba/_quarto-fw.yml missing %q:\n%s", want, fw)
		}
	}
	if strings.Contains(string(fw), "dispatch_POL") {
		t.Errorf("suba/_quarto-fw.yml contains POL page:\n%s", fw)
	}

	handout, _ := os.ReadFile(filepath.Join(root, "suba", "_quarto-handout.yml"))
	for _, want := range []string{
		"- index.qmd", "- care/index.qmd", "- care/water.qmd",
		"- x.qmd", "- truck_FW.qmd", "- dispatch_POL.qmd",
	} {
		if !strings.Contains(string(handout), want) {
			t.Errorf("suba/_quarto-handout.yml missing %q:\n%s", want, handout)
		}
	}
}

// Moving a page from one subproject into another and reloading before
// writing chapters removes it from the source subproject's config and adds
// it to the destination's.
func TestMoveAcrossSubprojects(t *testing.T) {
	root := writeMultiFixture(t)
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Move("suba/x.qmd", "subb/index.qmd", 0); err != nil {
		t.Fatal(err)
	}

	tree2, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree2.WriteChapters(nil); err != nil {
		t.Fatal(err)
	}

	suba, _ := os.ReadFile(filepath.Join(root, "suba", "_quarto.yml"))
	if strings.Contains(string(suba), "- x.qmd") {
		t.Errorf("suba/_quarto.yml still lists moved page:\n%s", suba)
	}
	subb, _ := os.ReadFile(filepath.Join(root, "subb", "_quarto.yml"))
	if !strings.Contains(string(subb), "- x.qmd") {
		t.Errorf("subb/_quarto.yml missing moved page:\n%s", subb)
	}
}
