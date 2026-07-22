package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUpdateChaptersReplaces(t *testing.T) {
	src := []byte(`project:
  type: book
  output-dir: _book
book:
  title: Test Book # keep
  chapters:
    - index.qmd
    - old.qmd
format:
  html: default
`)
	got, err := UpdateChapters(src, []string{"index.qmd", "a.qmd", "b/index.qmd"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	for _, want := range []string{
		"type: book", "output-dir: _book", "title: Test Book # keep",
		"- index.qmd", "- a.qmd", "- b/index.qmd", "html: default",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "old.qmd") {
		t.Errorf("output still contains old.qmd:\n%s", s)
	}
}

func TestUpdateChaptersCreatesBook(t *testing.T) {
	got, err := UpdateChapters([]byte("format:\n  pdf: default\n"), []string{"index.qmd"})
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, "book:") || !strings.Contains(s, "- index.qmd") || !strings.Contains(s, "pdf: default") {
		t.Errorf("unexpected output:\n%s", s)
	}
}

func TestUpdateChaptersEmptyDoc(t *testing.T) {
	got, err := UpdateChapters(nil, []string{"index.qmd"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "- index.qmd") {
		t.Errorf("unexpected output:\n%s", got)
	}
}

// WriteChapters updates _quarto.yml (which has book.chapters) with the full
// list and each selected book profile with the chapters of its own folder.
// A selected profile without a book key is left untouched.
func TestWriteChapters(t *testing.T) {
	root := writeFixture(t)
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.WriteChapters([]string{"chapter2", "web"}); err != nil {
		t.Fatal(err)
	}

	main, _ := os.ReadFile(filepath.Join(root, "_quarto.yml"))
	if !strings.Contains(string(main), "- chapter3/deep/leaf.qmd") {
		t.Errorf("_quarto.yml not updated:\n%s", main)
	}
	if !strings.Contains(string(main), "title: Test Book # keep") {
		t.Errorf("_quarto.yml lost content:\n%s", main)
	}

	ch2, _ := os.ReadFile(filepath.Join(root, "_quarto-chapter2.yml"))
	for _, want := range []string{"- chapter2/index.qmd", "- chapter2/second.qmd", "- chapter2/fourth.qmd"} {
		if !strings.Contains(string(ch2), want) {
			t.Errorf("_quarto-chapter2.yml missing %q:\n%s", want, ch2)
		}
	}
	for _, stray := range []string{"chapter3", "- index.qmd"} {
		if strings.Contains(string(ch2), stray) {
			t.Errorf("_quarto-chapter2.yml contains %q from outside its folder:\n%s", stray, ch2)
		}
	}

	ch3, _ := os.ReadFile(filepath.Join(root, "_quarto-chapter3-pol.yml"))
	if strings.Contains(string(ch3), "chapter3.qmd") {
		t.Errorf("_quarto-chapter3-pol.yml should be untouched (not selected):\n%s", ch3)
	}

	web, _ := os.ReadFile(filepath.Join(root, "_quarto-web.yml"))
	if strings.Contains(string(web), "chapters") {
		t.Errorf("_quarto-web.yml should be untouched (no book key):\n%s", web)
	}
}

// A flavor profile (<folder>-pol, <folder>-fw) gets the chapters of its base
// folder, including the folder's section page.
func TestWriteChaptersFlavorProfile(t *testing.T) {
	root := writeFixture(t)
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.WriteChapters([]string{"chapter3-pol"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "_quarto-chapter3-pol.yml"))
	for _, want := range []string{"- chapter3.qmd", "- chapter3/another.qmd", "- chapter3/deep/leaf.qmd"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("_quarto-chapter3-pol.yml missing %q:\n%s", want, got)
		}
	}
	for _, stray := range []string{"chapter2", "- index.qmd"} {
		if strings.Contains(string(got), stray) {
			t.Errorf("_quarto-chapter3-pol.yml contains %q from outside its folder:\n%s", stray, got)
		}
	}
}

func TestProfileTarget(t *testing.T) {
	tests := []struct {
		name, dir string
		markers   map[string]bool
	}{
		{"sysadmin", "sysadmin", map[string]bool{}},
		{"calltaker-pol", "calltaker", map[string]bool{markerPOL: true}},
		{"calltaker-fw", "calltaker", map[string]bool{markerFW: true}},
		{"calltaker-pol-fw", "calltaker", map[string]bool{markerFW: true, markerPOL: true}},
		{"netfw", "netfw", map[string]bool{}}, // no "-" boundary, not a flavor suffix
	}
	for _, tt := range tests {
		dir, markers := profileTarget(tt.name)
		if dir != tt.dir || !reflect.DeepEqual(markers, tt.markers) {
			t.Errorf("profileTarget(%q) = %q, %v, want %q, %v",
				tt.name, dir, markers, tt.dir, tt.markers)
		}
	}
}

// A flavored profile drops pages marked for the other flavor, including
// whole subtrees that inherit their marker from a _FW/_POL folder.
// Unmarked pages appear in both flavors.
func TestWriteChaptersFlavorFiltering(t *testing.T) {
	root := t.TempDir()
	pages := map[string]string{
		"index.qmd":                     "---\ntitle: Home\norder: 1\n---\n# Home\n",
		"calltaker/index.qmd":           "---\ntitle: Calltaker\norder: 2\n---\n# CT\n",
		"calltaker/base.qmd":            "---\ntitle: Base\norder: 1\n---\n# Base\n",
		"calltaker/alarm_FW.qmd":        "---\ntitle: Alarm\norder: 2\n---\n# Alarm\n",
		"calltaker/arrest_POL.qmd":      "---\ntitle: Arrest\norder: 3\n---\n# Arrest\n",
		"calltaker/drills_FW/index.qmd": "---\ntitle: Drills\norder: 4\n---\n# Drills\n",
		"calltaker/drills_FW/one.qmd":   "---\ntitle: One\norder: 1\n---\n# One\n",
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
	for _, name := range []string{"_quarto-calltaker-pol.yml", "_quarto-calltaker-fw.yml"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("book:\n  chapters:\n    - index.qmd\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.WriteChapters([]string{"calltaker-pol", "calltaker-fw"}); err != nil {
		t.Fatal(err)
	}

	pol, _ := os.ReadFile(filepath.Join(root, "_quarto-calltaker-pol.yml"))
	for _, want := range []string{"- calltaker/index.qmd", "- calltaker/base.qmd", "- calltaker/arrest_POL.qmd"} {
		if !strings.Contains(string(pol), want) {
			t.Errorf("pol profile missing %q:\n%s", want, pol)
		}
	}
	for _, stray := range []string{"alarm_FW", "drills_FW"} {
		if strings.Contains(string(pol), stray) {
			t.Errorf("pol profile contains FW page %q:\n%s", stray, pol)
		}
	}

	fw, _ := os.ReadFile(filepath.Join(root, "_quarto-calltaker-fw.yml"))
	for _, want := range []string{
		"- calltaker/index.qmd", "- calltaker/base.qmd", "- calltaker/alarm_FW.qmd",
		"- calltaker/drills_FW/index.qmd", "- calltaker/drills_FW/one.qmd",
	} {
		if !strings.Contains(string(fw), want) {
			t.Errorf("fw profile missing %q:\n%s", want, fw)
		}
	}
	if strings.Contains(string(fw), "arrest_POL") {
		t.Errorf("fw profile contains POL page:\n%s", fw)
	}
}

// _quarto.yml without a book section (e.g. a website project) is left alone.
func TestWriteChaptersSkipsNonBookMain(t *testing.T) {
	root := writeFixture(t)
	orig := "project:\n  type: website\n"
	os.WriteFile(filepath.Join(root, "_quarto.yml"), []byte(orig), 0o644)
	tree, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.WriteChapters(nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "_quarto.yml"))
	if string(got) != orig {
		t.Errorf("_quarto.yml changed:\n%s", got)
	}
}
