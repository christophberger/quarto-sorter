# Quarto Sorter

A local web app for sorting the pages of a [Quarto](https://quarto.org)
project in a tree view — by drag and drop.

Quarto keeps chapter order in three places that easily drift apart: the
`order` frontmatter field of each page (HTML rendering), the `book.chapters`
lists in `_quarto.yml` and profile configs (book rendering), and the Markdown
heading levels inside each file (which do not follow a page when it moves up
or down the folder hierarchy). Quarto Sorter keeps all three in sync.

## Usage

```sh
go run github.com/christophberger/quarto-sorter@latest [path/to/project]
```

Or download a prebuilt binary for your platform from the
[Releases page](https://github.com/christophberger/quarto-sorter/releases).
Check the version of an installed binary with `quarto-sorter --version`.

Then open http://localhost:8199 (change with `-addr`). Enter a project path
in the top bar, or pass it as an argument.

- The left pane shows the page tree, sorted by the `order` frontmatter.
  Pages without an `order` field are flagged with ⚠ and listed below their
  ordered siblings.
- Drag a page by its ⠿ handle to reorder it — or drop it onto another
  chapter to move it there (files are moved on disk accordingly).
- Click a page title to view the file in the right pane.
- ＋ buttons create pages; 🗑 moves a page to the system trash
  (falling back to a `_trash` directory inside the project).

## What a drop changes

After every drag and drop, Quarto Sorter updates:

1. **`order` fields** — the destination sibling group is renumbered 1…n to
   match what you see, so previously unordered pages there become ordered.
   In the group the page left, ordered pages close the gap and unordered
   pages stay unordered.
2. **Files on disk** — moving a page under another chapter moves the
   `.qmd` file (and, for a section, its whole directory) into the parent's
   directory. `chapter/index.qmd`-style and `chapter.qmd`-plus-directory
   sections are both supported.
3. **Heading levels** — when a page's depth changes, all Markdown headings
   in the affected files shift by the depth difference (clamped to
   `#`…`######`; code fences are left alone). This compensates for Quarto
   counting only Markdown heading levels, not folder depth.
4. **Chapter lists** — the `book.chapters` list is rewritten (in tree
   order, depth first) in every profile selected in the top bar
   (`_quarto-<name>.yml`), and in `_quarto.yml` itself if it already has
   one. Deselect a profile to leave its config untouched.

The tree — that is, the filesystem plus the `order` frontmatter — is the
single source of truth; chapter lists are generated from it.

## Development

Go 1.24+, no build tooling required; htmx and SortableJS are vendored in
`web/static/` and embedded into the binary.

```sh
go test ./...
go run .
```
