package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html/template"
	iofs "io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/christophberger/quarto-sorter/internal/project"
)

//go:embed web
var web embed.FS

// server holds the currently open project and the profile selection.
// A mutex serializes all access: this is a single-user local tool.
type server struct {
	mux  *http.ServeMux
	tmpl *template.Template

	mu       sync.Mutex
	root     string
	profiles []string
	selected map[string]bool

	// fp is the on-disk fingerprint of the project state that was last
	// rendered; /watch re-renders only when the disk no longer matches.
	fp string

	// prefsFile persists the profile selection per project root across
	// restarts; empty disables persistence. saved holds its content.
	prefsFile string
	saved     map[string][]string
}

// defaultPrefsFile returns the profile selections file in the user's config
// directory, or "" if no config directory is available.
func defaultPrefsFile() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "quarto-sorter", "profiles.json")
}

func newServer(prefsFile string) (*server, error) {
	funcs := template.FuncMap{
		"group": func(parent string, pages []*project.Page) any {
			return struct {
				Parent string
				Pages  []*project.Page
			}{parent, pages}
		},
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(web, "web/templates/*.tmpl")
	if err != nil {
		return nil, err
	}
	s := &server{
		mux: http.NewServeMux(), tmpl: tmpl, selected: map[string]bool{},
		prefsFile: prefsFile, saved: map[string][]string{},
	}
	if prefsFile != "" {
		// A missing or unreadable file just means no saved selections yet.
		if b, err := os.ReadFile(prefsFile); err == nil {
			json.Unmarshal(b, &s.saved)
		}
	}
	static, err := iofs.Sub(web, "web/static")
	if err != nil {
		return nil, err
	}
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	s.mux.HandleFunc("GET /{$}", s.page)
	s.mux.HandleFunc("POST /open", s.open)
	s.mux.HandleFunc("POST /profiles", s.setProfiles)
	s.mux.HandleFunc("GET /tree", s.treeHandler)
	s.mux.HandleFunc("GET /watch", s.watch)
	s.mux.HandleFunc("POST /move", s.move)
	s.mux.HandleFunc("POST /create", s.create)
	s.mux.HandleFunc("POST /delete", s.delete)
	s.mux.HandleFunc("GET /content", s.content)
	s.mux.HandleFunc("POST /save", s.save)
	return s, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// state bundles everything the page templates need.
type state struct {
	Root     string
	Profiles []string
	Selected map[string]bool
	Tree     *project.Tree
	Error    string
}

// load builds the current template state; the caller must hold s.mu.
func (s *server) load() (state, error) {
	st := state{Root: s.root, Profiles: s.profiles, Selected: s.selected}
	if s.root == "" {
		return st, nil
	}
	tree, err := project.Load(s.root)
	st.Tree = tree
	return st, err
}

func (s *server) render(w http.ResponseWriter, name string, data any) {
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// fingerprint hashes everything the tree pane and profile row are
// rendered from: the paths, sizes, and mtimes of the project's .qmd files
// and the _quarto*.yml configs at the root and in immediate subfolders
// (subprojects in multiproject mode). Equal fingerprints mean no refresh
// is needed.
func fingerprint(root string) string {
	h := fnv.New64a()
	filepath.WalkDir(root, func(p string, d iofs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries just drop out of the hash
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		qmd := strings.HasSuffix(name, ".qmd") && !strings.HasPrefix(name, "_")
		dir := filepath.Dir(p)
		cfg := (dir == root || filepath.Dir(dir) == root) && strings.HasSuffix(name, ".yml") &&
			(name == "_quarto.yml" || strings.HasPrefix(name, "_quarto-"))
		if !qmd && !cfg {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			fmt.Fprintf(h, "%s|%d|%d;", p, fi.Size(), fi.ModTime().UnixNano())
		}
		return nil
	})
	return strconv.FormatUint(h.Sum64(), 16)
}

// rememberFP records the current on-disk fingerprint as rendered, so
// /watch stays quiet until something changes outside the responses we
// produce ourselves. The caller must hold s.mu.
func (s *server) rememberFP() {
	if s.root != "" {
		s.fp = fingerprint(s.root)
	}
}

// refreshProfiles rescans the project's book profiles, keeping the
// selection of profiles it already knows. A newly appearing profile is
// selected if it is part of the project's saved selection, or if the user
// never customized the selection (matching the select-all default).
// It reports whether the profile row changed. The caller must hold s.mu.
func (s *server) refreshProfiles() bool {
	if s.root == "" {
		return false
	}
	profiles, err := project.Profiles(s.root)
	if err != nil {
		return false
	}
	saved, customized := s.saved[s.root]
	selected := map[string]bool{}
	for _, p := range profiles {
		if slices.Contains(s.profiles, p) {
			selected[p] = s.selected[p]
		} else {
			selected[p] = !customized || slices.Contains(saved, p)
		}
	}
	changed := !slices.Equal(profiles, s.profiles)
	s.profiles, s.selected = profiles, selected
	return changed
}

func (s *server) page(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshProfiles()
	s.rememberFP()
	st, err := s.load()
	if err != nil {
		st.Error = err.Error()
	}
	s.render(w, "page", st)
}

func (s *server) open(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := filepath.Abs(strings.TrimSpace(r.FormValue("path")))
	if err == nil {
		var fi os.FileInfo
		if fi, err = os.Stat(root); err == nil && !fi.IsDir() {
			err = fmt.Errorf("%s is not a directory", root)
		}
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.setRoot(root); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.rememberFP()
	st, err := s.load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.render(w, "main", st)
	s.renderProfilesOOB(w, st)
}

// renderProfilesOOB appends an out-of-band refresh of the profile
// checkbox row in the header. The caller must hold s.mu.
func (s *server) renderProfilesOOB(w http.ResponseWriter, st state) {
	fmt.Fprint(w, `<div hx-swap-oob="innerHTML:#profiles-form">`)
	s.render(w, "profiles", st)
	fmt.Fprint(w, `</div>`)
}

// setRoot switches to the project at root, restoring the saved profile
// selection for that project, or selecting all profiles if none was saved.
// The caller must hold s.mu (or not be serving yet).
func (s *server) setRoot(root string) error {
	profiles, err := project.Profiles(root)
	if err != nil {
		return err
	}
	s.root, s.profiles, s.selected = root, profiles, map[string]bool{}
	if saved, ok := s.saved[root]; ok {
		// Restore the saved selection, dropping profiles that no longer exist.
		for _, p := range saved {
			if slices.Contains(profiles, p) {
				s.selected[p] = true
			}
		}
		return nil
	}
	for _, p := range profiles {
		s.selected[p] = true // default: keep all profiles in sync
	}
	return nil
}

func (s *server) setProfiles(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r.ParseForm()
	s.selected = map[string]bool{}
	for _, p := range r.Form["profile"] {
		s.selected[p] = true
	}
	if s.root != "" {
		sel := []string{} // non-nil: an empty selection is a valid saved state
		for _, p := range s.profiles {
			if s.selected[p] {
				sel = append(sel, p)
			}
		}
		s.saved[s.root] = sel
		s.savePrefs()
	}
	w.WriteHeader(http.StatusNoContent)
}

// savePrefs writes the per-project profile selections to the prefs file.
// Persistence is best effort: the in-memory state is already updated, so a
// write failure only loses the selection across restarts. The caller must
// hold s.mu.
func (s *server) savePrefs() {
	if s.prefsFile == "" {
		return
	}
	b, err := json.MarshalIndent(s.saved, "", "  ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.prefsFile), 0o755); err != nil {
		return
	}
	os.WriteFile(s.prefsFile, b, 0o644)
}

func (s *server) treeHandler(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renderTree(w, "")
}

// renderTree renders the tree fragment, prefixed with an error banner if
// msg is non-empty. If the set of book profiles changed on disk, the
// profile row in the header is refreshed out of band along with the tree.
// The caller must hold s.mu.
func (s *server) renderTree(w http.ResponseWriter, msg string) {
	profilesChanged := s.refreshProfiles()
	s.rememberFP()
	st, err := s.load()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if st.Tree == nil {
		http.Error(w, "no project open", http.StatusBadRequest)
		return
	}
	st.Error = msg
	s.render(w, "treewrap", st)
	if profilesChanged {
		s.renderProfilesOOB(w, st)
	}
}

// watch is polled by the client. It re-renders the tree (and, when
// needed, the profile row) only when the project changed on disk outside
// the sorter; 204 tells htmx to leave the page alone.
func (s *server) watch(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == "" || fingerprint(s.root) == s.fp {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.renderTree(w, "")
}

// apply runs op on a freshly loaded tree, syncs the chapter lists of the
// selected profiles, and responds with the updated tree. Errors from op
// appear as a banner above the (reverted) tree.
func (s *server) apply(w http.ResponseWriter, op func(*project.Tree) error) {
	tree, err := project.Load(s.root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := op(tree); err != nil {
		s.renderTree(w, err.Error())
		return
	}
	if tree, err = project.Load(s.root); err == nil {
		var selected []string
		for p, on := range s.selected {
			if on {
				selected = append(selected, p)
			}
		}
		err = tree.WriteChapters(selected)
	}
	if err != nil {
		s.renderTree(w, err.Error())
		return
	}
	s.renderTree(w, "")
}

func (s *server) move(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos, err := strconv.Atoi(r.FormValue("pos"))
	if err != nil {
		http.Error(w, "bad pos", http.StatusBadRequest)
		return
	}
	s.apply(w, func(t *project.Tree) error {
		return t.Move(r.FormValue("src"), r.FormValue("parent"), pos)
	})
}

func (s *server) create(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := r.FormValue("name")
	if name == "" {
		name = r.Header.Get("HX-Prompt") // per-node ＋ button
	}
	title := r.FormValue("title")
	if title == "" {
		title = name
	}
	// The top-bar form inserts after the page selected in the tree (its
	// path travels as "after"); the per-node ＋ button appends a child to
	// its parent instead.
	parent, after := r.FormValue("parent"), r.FormValue("after")
	s.apply(w, func(t *project.Tree) error {
		var err error
		if parent == "" && after != "" {
			_, err = t.CreatePageAfter(after, name, title)
		} else {
			_, err = t.CreatePage(parent, name, title)
		}
		return err
	})
}

func (s *server) delete(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apply(w, func(t *project.Tree) error {
		return t.DeletePage(r.FormValue("path"))
	})
}

// resolvePath validates rel as a page path relative to the open project and
// returns its absolute location on disk. The caller must hold s.mu.
func (s *server) resolvePath(rel string) (string, error) {
	if s.root == "" {
		return "", fmt.Errorf("no project open")
	}
	if clean := path.Clean(rel); clean != rel || path.IsAbs(rel) || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid path")
	}
	return filepath.Join(s.root, filepath.FromSlash(rel)), nil
}

func (s *server) content(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel := r.URL.Query().Get("path")
	abs, err := s.resolvePath(rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	title := project.ParseFrontmatter(body).Title
	s.render(w, "content", struct {
		Title, Path, Body string
	}{title, rel, string(body)})
	// The reload button also refreshes the tree: outside edits may have
	// changed titles or the chapter order.
	if r.URL.Query().Get("reload") != "" {
		s.renderTreeOOB(w)
	}
}

// save writes the edited body back to an existing page. The editor pane is
// left untouched so autosave never steals the cursor; only the heading is
// updated out of band, plus the tree if the title changed.
func (s *server) save(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel := r.FormValue("path")
	abs, err := s.resolvePath(rel)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	old, err := os.ReadFile(abs)
	if err != nil {
		http.Error(w, "no such page", http.StatusBadRequest)
		return
	}
	body := []byte(r.FormValue("body"))
	if err := os.WriteFile(abs, body, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	title := project.ParseFrontmatter(body).Title
	fmt.Fprint(w, `<h2 class="content-title" id="content-title" hx-swap-oob="true">`)
	s.render(w, "content-heading", struct{ Title, Path string }{title, rel})
	fmt.Fprint(w, `</h2>`)
	if title != project.ParseFrontmatter(old).Title {
		s.renderTreeOOB(w)
	}
	// Our own write must not look like an outside change to /watch.
	s.rememberFP()
}

// renderTreeOOB appends an out-of-band refresh of the tree pane to the
// response. The caller must hold s.mu.
func (s *server) renderTreeOOB(w http.ResponseWriter) {
	profilesChanged := s.refreshProfiles()
	s.rememberFP()
	if st, err := s.load(); err == nil && st.Tree != nil {
		fmt.Fprint(w, `<div hx-swap-oob="innerHTML:#tree">`)
		s.render(w, "treewrap", st)
		fmt.Fprint(w, `</div>`)
		if profilesChanged {
			s.renderProfilesOOB(w, st)
		}
	}
}
