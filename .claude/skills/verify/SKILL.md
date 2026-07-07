---
name: verify
description: Build, run, and drive Quarto Sorter end-to-end to verify changes at the browser surface.
---

# Verifying Quarto Sorter

1. Create a fixture Quarto project in a temp dir: a few `.qmd` files with
   `title`/`order` frontmatter (nest some under `chapter2/` with an
   `index.qmd`), plus a minimal `_quarto.yml` with a `book.chapters` list.
2. Start the app on it: `go run . -addr localhost:8199 <fixture-dir>`
   (background). It serves plain HTML + htmx; `curl --noproxy '*'
   localhost:8199/` already shows the rendered tree for cheap assertions.
3. For the real surface (htmx swaps, SortableJS drag), drive headless
   Chromium with Playwright (`npm install playwright` in a scratch dir;
   launch with `executablePath: '/opt/pw-browsers/chromium-1194/chrome-linux/chrome'`).

Gotchas:

- After clicking a page title, wait until `#content input[name="path"]`
  equals the clicked path before touching the textarea — filling earlier
  races the incoming htmx swap and gets clobbered.
- After a save, the editor already shows the saved page; clicking its
  tree title again re-triggers the same race.
- Playwright's `dragTo` does not move SortableJS items; use manual
  `mouse.down()` + stepped `mouse.move()` + `mouse.up()` and wait for the
  `/move` response.
- Tree refreshes from `/save` arrive as htmx out-of-band swaps
  (`htmx:oobAfterSwap`), not `htmx:afterSwap`.
