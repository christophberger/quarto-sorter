// Quarto Sorter - app behavior (plain ES6, no build step)

var sortableInstances = [];

// Collapsed branches, keyed by each node's data-key. #tree is fully
// re-rendered on every move/create/delete/save, so this module-level state
// is what keeps collapsed branches collapsed across those re-renders.
var collapsed = new Set();

// applyCollapsed re-applies the persisted collapsed state to the freshly
// rendered tree.
function applyCollapsed(tree) {
  tree.querySelectorAll('li.page.has-children').forEach(function (li) {
    li.classList.toggle('collapsed', collapsed.has(li.dataset.key));
  });
}

function initTree() {
  // Destroy any stale Sortable instances before re-initializing.
  sortableInstances.forEach(function (inst) {
    inst.destroy();
  });
  sortableInstances = [];

  var tree = document.getElementById('tree');
  if (!tree) {
    return;
  }

  applyCollapsed(tree);

  var lists = tree.querySelectorAll('ul.children');
  lists.forEach(function (list) {
    var inst = Sortable.create(list, {
      group: 'pages',
      handle: '.drag-handle',
      animation: 150,
      fallbackOnBody: true,
      swapThreshold: 0.65,
      // Inverted swap makes the outer band of a row insert next to it, so
      // hovering the lower edge of the last subentry (in the gutter left of
      // its child list) inserts AFTER it — the "1.2" drop position that a
      // plain swap zone never offers with nested lists.
      invertSwap: true,
      invertedSwapThreshold: 0.65,
      // Only treat a list as an empty drop target when the pointer is right
      // inside it, so the empty child list under the last row does not grab
      // drops meant for the parent list's bottom strip.
      emptyInsertThreshold: 3,
      ghostClass: 'drag-ghost',
      onEnd: function (evt) {
        var sameList = evt.from === evt.to;
        var sameIndex = evt.oldIndex === evt.newIndex;
        if (sameList && sameIndex) {
          return;
        }

        var src = evt.item.dataset.path;
        var parent = evt.to.dataset.parent;
        var pos = evt.newIndex;

        htmx.ajax('POST', '/move', {
          target: '#tree',
          swap: 'innerHTML',
          values: { src: src, parent: parent, pos: pos }
        });
      }
    });
    sortableInstances.push(inst);
  });
}

// currentPath is the page open in the editor; applySelection re-highlights
// it after every tree re-render (moves, saves, reloads).
var currentPath = null;

function applySelection() {
  document.querySelectorAll('#tree li.page').forEach(function (li) {
    li.classList.toggle('selected', !!currentPath && li.dataset.path === currentPath);
  });
}

document.addEventListener('DOMContentLoaded', function () {
  initTree();
});

document.body.addEventListener('htmx:afterSwap', function (evt) {
  var id = evt.detail && evt.detail.target && evt.detail.target.id;
  if (id === 'tree' || id === 'main') { // /open swaps #main, everything else #tree
    initTree();
    applySelection();
  }
  if (id === 'content') { // track whatever the editor now shows
    var input = evt.detail.target.querySelector('input[name="path"]');
    currentPath = input ? input.value : null;
    applySelection();
  }
});

// Out-of-band swaps (e.g. /save refreshing #tree alongside #content) fire
// htmx:oobAfterSwap instead of htmx:afterSwap, so Sortable needs its own hook.
document.body.addEventListener('htmx:oobAfterSwap', function (evt) {
  var id = evt.detail && evt.detail.target && evt.detail.target.id;
  if (id === 'tree') {
    initTree();
    applySelection();
  }
});

document.body.addEventListener('click', function (evt) {
  var toggle = evt.target.closest('.toggle');
  if (toggle) {
    var node = toggle.closest('li.page');
    if (node && node.classList.contains('has-children')) {
      var key = node.dataset.key;
      if (collapsed.has(key)) {
        collapsed.delete(key);
      } else {
        collapsed.add(key);
      }
      node.classList.toggle('collapsed', collapsed.has(key));
    }
    return;
  }

  var link = evt.target.closest('a.title');
  if (!link) {
    return;
  }
  var item = link.closest('li');
  currentPath = item ? item.dataset.path : null;
  applySelection();
});

// Autosave feedback: the edit form posts /save with hx-swap="none", so the
// only visible trace is the status text next to the heading.
function setSaveStatus(text) {
  var el = document.getElementById('save-status');
  if (el) {
    el.textContent = text;
  }
}

document.body.addEventListener('htmx:beforeRequest', function (evt) {
  var elt = evt.detail && evt.detail.elt;
  if (elt && elt.classList && elt.classList.contains('edit-form')) {
    setSaveStatus('Saving…');
  }
});

document.body.addEventListener('htmx:afterRequest', function (evt) {
  var elt = evt.detail && evt.detail.elt;
  if (elt && elt.classList && elt.classList.contains('edit-form')) {
    setSaveStatus(evt.detail.successful ? 'Saved' : 'Save failed');
  }
});
