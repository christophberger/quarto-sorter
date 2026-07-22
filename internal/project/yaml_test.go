package project

import (
	"os"
	"path/filepath"
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

func TestProfileDir(t *testing.T) {
	for name, want := range map[string]string{
		"sysadmin":         "sysadmin",
		"calltaker-pol":    "calltaker",
		"calltaker-fw":     "calltaker",
		"calltaker-pol-fw": "calltaker",
		"netfw":            "netfw", // no "-" boundary, not a flavor suffix
	} {
		if got := profileDir(name); got != want {
			t.Errorf("profileDir(%q) = %q, want %q", name, got, want)
		}
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
