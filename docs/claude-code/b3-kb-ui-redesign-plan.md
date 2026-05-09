# B3 — Knowledge Base UI Redesign + Route Move

> **Status:** Plan finalized. Will be saved to `docs/claude-code/b3-kb-ui-redesign-plan.md` once plan-mode is exited.
> **Scope:** UI/route layer only. No backend changes. No new primitives. No new packages (only removals).

## Context

The B3 backend + scaffold UI shipped, but the UI lives in the wrong place and looks broken:
1. Routes are nested under `/admin/knowledge`. KB authoring should be a **top-level destination** like Inbox or Contacts.
2. The list page is half-formed — folder column is a 256px aside instead of a sidebar, "New article" button has no visible affordance, no filter bar, no per-row actions, no empty-state CTA, AdminSplitLayout's "Help" panel floats disconnected.
3. The form page stacks metadata above the editor, pushing the work surface below the fold.
4. `KbArticleEditor.vue` toolbar wraps to two rows because it carries CS-irrelevant marks (Highlight / Sub / Sup).

This plan moves the routes, restructures the list and form pages to match Libredesk's existing patterns (shadcn Badge, DataTable, PageHeader card-frame), and trims the editor.

---

## 1. Discovery findings

### Main left rail — `IconRail.vue` (NOT data-driven)
[components/sidebar/IconRail.vue](frontend/apps/main/src/components/sidebar/IconRail.vue) — every entry is a hardcoded `<Tooltip><TooltipTrigger as-child><Button as-child variant="ghost"><router-link>…</router-link></Button></TooltipTrigger></Tooltip>` block. There is no constants file driving it. Existing entries:
- Inbox ([line 11-26](frontend/apps/main/src/components/sidebar/IconRail.vue#L11-L26)) → `lastInboxPath || { name: 'inboxes' }`, `Inbox` icon
- Contacts ([line 28-43](frontend/apps/main/src/components/sidebar/IconRail.vue#L28-L43)) → `userStore.can('contacts:read_all')`, `BookUser` icon
- Reports ([line 45-60](frontend/apps/main/src/components/sidebar/IconRail.vue#L45-L60)) → `userStore.hasReportTabPermissions`, `FileLineChart` icon
- Admin ([line 62-81](frontend/apps/main/src/components/sidebar/IconRail.vue#L62-L81)) → `userStore.hasAdminTabPermissions`, `Shield` icon

Lucide icons imported at [line 105](frontend/apps/main/src/components/sidebar/IconRail.vue#L105): `Inbox, Shield, FileLineChart, BookUser`. **`BookOpen` is NOT imported here yet** but is imported in [Sidebar.vue](frontend/apps/main/src/components/sidebar/Sidebar.vue) (added during B3 scaffold) — we add it to IconRail's lucide import block.

**Stop-condition resolved:** the rail is in the same template-by-permission-`v-if` shape as the secondary `Sidebar.vue`/`navigation.js` filtering, just hardcoded inline rather than data-driven. Adding a "Knowledge" entry is a 16-line block.

### Top-level destination reference
[views/contact/ContactsView.vue](frontend/apps/main/src/views/contact/ContactsView.vue) is 9 lines — wraps a `ContactList` layout around a `ContactsList` feature. Not useful for a table-based listing. Better reference: **macros**'s split between `views/admin/macros/MacroList.vue` (table) and the `Macros.vue` parent. Authentic top-level reference is **the contact directory** ([layouts/contact/ContactList.vue](frontend/apps/main/src/layouts/contact/ContactList.vue)) — it sits inside the `App.vue` PageHeader card frame.

### App.vue card frame
[App.vue:42](frontend/apps/main/src/App.vue#L42) renders `<PageHeader />` inside the card frame for every non-inbox route. So a top-level `/knowledge` automatically gets the "Knowledge base" title from `route.meta.titleKey`. We don't render our own page-title `<h1>`.

### PageHeader.vue
[components/layout/PageHeader.vue](frontend/apps/main/src/components/layout/PageHeader.vue) is a tight component: sidebar-toggle button + i18n-keyed title + separator. **No description, no actions slot.** Brief calls for "one-line description below the title + New article button on the right" — that's part of the **content area**, not the PageHeader. Implementation: render a sub-header strip at the top of `KnowledgeList.vue` for description + actions.

### Router structure
[router/index.js](frontend/apps/main/src/router/index.js) — all routes in one file. Three groups:
- `OuterApp` (auth, lines 11-34)
- `App` (everything authenticated, lines 35+) — top-level children include `contacts` ([line 40](frontend/apps/main/src/router/index.js#L40)), `/reports/*` ([line 52](frontend/apps/main/src/router/index.js#L52)), `/inboxes/*`, `/admin/*` (with KB nested inside admin's children today)
- catch-all → `/inboxes/assigned`

The current `/admin/knowledge` block lives inside the admin children. Moving it out means lifting it up next to the `contacts` block.

### B3 KB Vue files (existing)
All read verbatim — no truncation:
- [views/admin/knowledge/Knowledge.vue](frontend/apps/main/src/views/admin/knowledge/Knowledge.vue) (3 lines, `<router-view/>`)
- [views/admin/knowledge/KnowledgeList.vue](frontend/apps/main/src/views/admin/knowledge/KnowledgeList.vue) (268 lines) — `AdminSplitLayout`, hand-rolled aside w-64, hand-rolled flex
- [views/admin/knowledge/CreateArticle.vue](frontend/apps/main/src/views/admin/knowledge/CreateArticle.vue) (62 lines) — `max-w-4xl mx-auto`, hand-rolled `<h1>`
- [views/admin/knowledge/EditArticle.vue](frontend/apps/main/src/views/admin/knowledge/EditArticle.vue) (128 lines) — same shape; Publish/Unpublish via slot=header-actions
- [features/admin/knowledge/ArticleForm.vue](frontend/apps/main/src/features/admin/knowledge/ArticleForm.vue) (191 lines) — single-column `space-y-6`; **status pill uses `bg-green-100 text-green-800`** ([line 7](frontend/apps/main/src/features/admin/knowledge/ArticleForm.vue#L7)), violates the no-hardcoded-color token rule
- [features/admin/knowledge/FolderTree.vue](frontend/apps/main/src/features/admin/knowledge/FolderTree.vue), [FolderTreeNode.vue](frontend/apps/main/src/features/admin/knowledge/FolderTreeNode.vue), [FolderPicker.vue](frontend/apps/main/src/features/admin/knowledge/FolderPicker.vue), [FolderDialog.vue](frontend/apps/main/src/features/admin/knowledge/FolderDialog.vue) — all keep
- [features/admin/knowledge/KbArticleEditor.vue](frontend/apps/main/src/features/admin/knowledge/KbArticleEditor.vue) (260 lines) — single-row toolbar that wraps; carries Highlight/Sub/Sup
- [features/admin/knowledge/dataTableColumns.js](frontend/apps/main/src/features/admin/knowledge/dataTableColumns.js) — uses `format(..., 'PPpp')` for absolute time
- [features/admin/knowledge/dataTableDropdown.vue](frontend/apps/main/src/features/admin/knowledge/dataTableDropdown.vue) — keep, edit imports later

### Empty-state pattern
[components/datatable/DataTable.vue:87-96](frontend/apps/main/src/components/datatable/DataTable.vue#L87-L96) — `<Ghost class="h-8 w-8 text-muted-foreground/50" /> + <p>{{ emptyText }}</p>`. `emptyText` defaults to `t('globals.messages.noResultsFound')`. Inline-only, no slot to override it. **There is no shared `EmptyState` component.** To get the brief's "Lucide icon + heading + CTA", we render an empty-state block **outside DataTable** (above or in place of it) when `articles.length === 0 && !isLoading && noFiltersActive`.

### Relative-time formatter
**Not used anywhere in main app.** `date-fns` ^3.6.0 is in [package.json:52](frontend/package.json#L52) (used by `format` for absolute timestamps and by Calendar/DatePicker). The widget has a `useRelativeTime` composable; the main app does not. Going with `formatDistanceToNow` from `date-fns` is the no-new-dep path.

### Filter-bar pattern
Searched macros/agents/tags admin views. **None of them surface filter dropdowns** — they rely on DataTable's built-in `globalFilter` text search. Status/Locale filters are a new pattern for this codebase. Rather than invent a `FilterBar.vue` primitive, the simplest reuse is shadcn's `Select` + `Input` placed inline above the DataTable, controlling local refs that drive the API params.

### Status pill / Badge component
[shared-ui/components/ui/badge/index.js](frontend/shared-ui/components/ui/badge/index.js) — `cva` with variants: `default` (primary), `secondary` (muted), `destructive`, `outline`. **No `success` variant.** Mapping for KB:
- Published → `default` (primary brand color, will look right in both light + dark modes via tokens)
- Draft → `secondary` (muted gray)

(Brief said "green for Published". Strict green requires a new `success` variant — surfaced as open question. Using `default` matches existing precedent.)

### Pagination
[DataTable.vue](frontend/apps/main/src/components/datatable/DataTable.vue) does not render pagination controls. The TanStack table is mounted with `getCoreRowModel`/`getFilteredRowModel`/`getSortedRowModel` only — no `getPaginationRowModel`. Articles list relies on virtualization (`useVirtualizer` at [line 192](frontend/apps/main/src/components/datatable/DataTable.vue#L192)). **The whole loaded result set is rendered virtualized; there is no UI pagination.** For v1 we'll fetch with a sane page size (e.g., 100) and rely on virtualization. Adding pagination UI is out of scope (would be a primitive change to DataTable).

### Import-ripple count
`grep -rln "views/admin/knowledge\|features/admin/knowledge" frontend/apps/main/src/ | wc -l` → **4** files reference the path:
1. [router/index.js](frontend/apps/main/src/router/index.js)
2. [views/admin/knowledge/KnowledgeList.vue](frontend/apps/main/src/views/admin/knowledge/KnowledgeList.vue)
3. [views/admin/knowledge/CreateArticle.vue](frontend/apps/main/src/views/admin/knowledge/CreateArticle.vue)
4. [views/admin/knowledge/EditArticle.vue](frontend/apps/main/src/views/admin/knowledge/EditArticle.vue)

Internal imports inside `features/admin/knowledge/*` are all relative (`./FolderTreeNode.vue`, `./formSchema.js`, `./utils/slugify.js`, etc.) — **survive a folder rename without changes**.

---

## 2. File-move decision — **(a) MOVE**

Import-ripple count is **4**, well below the 15 threshold. Recommend the move:
- `views/admin/knowledge/` → `views/knowledge/`
- `features/admin/knowledge/` → `features/knowledge/`

Touched imports (4 sites):
- `router/index.js`: `@main/views/admin/knowledge/X.vue` → `@main/views/knowledge/X.vue` (4 paths)
- `views/knowledge/KnowledgeList.vue`: `@/features/admin/knowledge/X` → `@/features/knowledge/X` (5 imports)
- `views/knowledge/CreateArticle.vue`: same (1 import)
- `views/knowledge/EditArticle.vue`: same (1 import)

The legacy nav entry in `constants/navigation.js` is removed (B3 scaffold added it under Admin); `permissions.js` keys stay (constants are stable).

---

## 3. Files to be created

| Path | Purpose |
|---|---|
| `views/knowledge/KnowledgeList.vue` | Replaces `views/admin/knowledge/KnowledgeList.vue` (re-shape — see §4). Will be created via the move + rewrite. |
| `views/knowledge/Knowledge.vue` | Move of the parent shell. Identical 3-liner. |
| `views/knowledge/CreateArticle.vue` | Move + rewrite (top bar + breadcrumb + two-column body). |
| `views/knowledge/EditArticle.vue` | Move + rewrite (status sidebar holds Publish/Unpublish + status badge). |
| `features/knowledge/*` | Move of all 11 feature files. ArticleForm.vue and KbArticleEditor.vue rewritten in place per §4. |

No new components. No new primitives in `shared-ui/`. The empty state uses DataTable's built-in `<Ghost>` icon + `emptyText` (locked decision) — `EmptyArticles.vue` is NOT needed.

---

## 4. Files to be modified

### Routing & navigation
- **[router/index.js](frontend/apps/main/src/router/index.js)** —
  - Remove the `path: 'knowledge'` block under the admin group ([near line 470 area](frontend/apps/main/src/router/index.js)).
  - Add a sibling of `contacts` (around [line 40](frontend/apps/main/src/router/index.js#L40)):
    ```
    {
      path: '/knowledge',
      name: 'knowledge',
      component: () => import('@main/views/knowledge/Knowledge.vue'),
      redirect: { name: 'knowledge-list' },
      meta: { titleKey: 'globals.terms.knowledgeBase' },
      children: [
        { path: '', name: 'knowledge-list', component: () => import('@main/views/knowledge/KnowledgeList.vue') },
        { path: 'new', name: 'new-article', component: () => import('@main/views/knowledge/CreateArticle.vue'),  meta: { titleKey: 'admin.knowledge.actions.newArticle' } },
        { path: ':id/edit', name: 'edit-article', props: true, component: () => import('@main/views/knowledge/EditArticle.vue'), meta: { titleKey: 'admin.knowledge.actions.editArticle' } }
      ]
    }
    ```
  - Update the existing `name: 'edit-knowledge-article'` / `'new-knowledge-article'` references inside `dataTableDropdown.vue`, `KnowledgeList.vue`, `CreateArticle.vue`, `EditArticle.vue` to the new short names: `edit-article` / `new-article`.
  - **Add bookmark-redirect rule** (locked by user) at the same level — keeps existing teammate links working:
    ```
    { path: '/admin/knowledge', redirect: { name: 'knowledge-list' } },
    { path: '/admin/knowledge/articles/new', redirect: { name: 'new-article' } },
    { path: '/admin/knowledge/articles/:id/edit', redirect: (to) => ({ name: 'edit-article', params: { id: to.params.id } }) }
    ```

- **[constants/navigation.js](frontend/apps/main/src/constants/navigation.js)** — remove the B3 scaffold's `adminNavItems` entry for Knowledge (the trailing block under integrations).

- **[components/sidebar/IconRail.vue](frontend/apps/main/src/components/sidebar/IconRail.vue)** — add a new `<Tooltip>` block between Contacts and Reports (or at the end of the main group) for Knowledge:
  - Permission gate: `v-if="userStore.can('knowledge:read')"`
  - Active state: `route.path.startsWith('/knowledge')`
  - Icon: `BookOpen` (add to lucide import on line 105)
  - i18n: `t('globals.terms.knowledgeBase', 2)` for tooltip
  - Target: `{ name: 'knowledge-list' }`

### List page rewrite — `views/knowledge/KnowledgeList.vue`
- Drop `AdminSplitLayout` and the help-text slot.
- Top sub-header strip (rendered inside the App.vue card frame, beneath PageHeader): one-line description + primary "New article" button (right-aligned).
- Two-column body via `<div class="flex h-full">`:
  - **Folder sidebar** ~220px: `<aside class="w-[220px] shrink-0 border-r border-border flex flex-col">` containing:
    - "All articles" entry at top (no count per locked decision B1 = drop counts).
    - The existing `FolderTree.vue` (untouched — it already does the indented tree from `ListAllFolders`).
    - "+ New folder" button at the bottom of the aside, anchored.
  - **Article list** flex-1: filter bar (Search input · Status `Select` · Locale `Select`) above the DataTable, then `<DataTable :searchable="false">`. **DataTable's built-in `<Ghost>` empty state covers both no-articles-yet and no-results-match-filters cases** (locked decision); no custom empty-state component needed.
- Filter bar's Search input is **debounced** (~300ms) and re-fetches with `search=` param. DataTable's own `searchable` is set to `false` — single source of truth, server-side ILIKE.
- Pass filter values (search, status, locale) into `api.getKnowledgeArticles({ search, status, locale, folder_id })`. The backend handlers already accept these params.
- Build `folderPathById` map client-side from `folders.value` (recursive walk via `parent_id`) for the row's secondary line.
- Move/Rename/Delete folder dialogs stay as-is (already in current KnowledgeList.vue).

### List columns — `features/knowledge/dataTableColumns.js`
- Title cell becomes two-line:
  - Primary: `<span class="font-medium">{{ title }}</span>`
  - Secondary `<span class="text-xs text-muted-foreground">{{ folderPath }} · {{ locale }} {{ ai-grounded ? '· AI-grounded' : '' }}</span>`
- Status cell: replace inline Tailwind chips with `<Badge :variant="status === 'published' ? 'default' : 'secondary'">{{ t(`admin.knowledge.status.${status}`) }}</Badge>`. Locked by user.
- Type column unchanged (classification label).
- Updated column: switch from `format(..., 'PPpp')` to `formatDistanceToNow(date, { addSuffix: true })` from `date-fns`.
- Folder column: removed (folder is now part of the title secondary line).
- Per-row dropdown column unchanged.

### Form page rewrite — `features/knowledge/ArticleForm.vue` + view wrappers
- ArticleForm body becomes `grid grid-cols-12 gap-6`:
  - **Editor column** (`col-span-8`): Title input as a borderless XL `<input>` (no `<FormLabel>Title</FormLabel>`, placeholder = "Untitled article"); below it, `KbArticleEditor.vue`.
  - **Metadata sidebar** (`col-span-4`): `<aside class="space-y-4">` with these sections in order:
    1. Status: Badge ("Draft"/"Published") + Publish/Unpublish button (uses existing publish/unpublish endpoints).
    2. Folder: existing `FolderPicker.vue`, with label.
    3. Slug: `<Input>` with helper text.
    4. Locale: `<select>` (kept simple; matches existing pattern).
    5. Classification: `<select>` (same).
    6. AI-groundable: checkbox + label + description.
- Status badge inside ArticleForm replaces the hand-rolled `bg-green-100 text-green-800` with `<Badge :variant="...">` — fixes the existing token violation.
- Slugify-on-title-input behavior preserved.
- `getBody(editor)` fresh-on-submit pattern preserved.

### View wrappers — `views/knowledge/CreateArticle.vue` / `EditArticle.vue`
Top bar (above ArticleForm) becomes:
- Left: back button (`<router-link :to="{name:'knowledge-list'}"><ArrowLeft/></router-link>`) + breadcrumb. Per locked decision B5: `Knowledge › New article` until folder picked, `Knowledge › <folder> › New article` after. For Edit: `Knowledge › <folder> › <article title>`. Built from a small inline computed.
- Right: Cancel + Save buttons.
- No autosave indicator (locked B4 = drop).

Drop the `max-w-4xl mx-auto` container — the App.vue card frame already supplies the canvas. Use `<div class="px-6 py-6 h-full flex flex-col gap-6">` for the wrapper.

Move Publish/Unpublish out of `#header-actions` slot and into the right-sidebar Status section inside ArticleForm.

### KbArticleEditor.vue toolbar trim
- Remove imports: `Highlight`, `Subscript`, `Superscript` (TipTap exts). Remove from the `extensions: [...]` array.
- Remove imports: `Highlighter`, `Subscript as SubscriptIcon`, `Superscript as SuperscriptIcon` (Lucide).
- Remove the three buttons (the "Mark" group).
- Remove the existing `<select>` for align; replace with a single `<Popover>` containing four align buttons (Left/Center/Right/Justify) — **add new lucide imports**: `AlignLeft`, `AlignCenter`, `AlignRight`, `AlignJustify`. Trigger button shows the current alignment icon. State driven by existing `alignValue` ref + existing `editor.chain().focus().setTextAlign(v).run()`.
- Result: 6 toolbar groups (History · Heading · Inline · List · Block · Link · Align-popover) — fits one row.

### Dependencies — `frontend/package.json`
Remove three lines:
- `"@tiptap/extension-highlight": "^2.11.2",`
- `"@tiptap/extension-subscript": "^2.11.2",`
- `"@tiptap/extension-superscript": "^2.11.2",`

Run `pnpm install` to update the lockfile.

---

## 5. Files to be deleted

None outright. The "delete" is the move (renames). Git surfaces the rename as deletion+addition; tooling tracks moves via similarity index.

---

## 6. Migration risk

| Risk | Mitigation |
|---|---|
| Bookmarked `/admin/knowledge*` URLs break | Three Vue Router `redirect` rules added (locked by user). One-line each. |
| Admin sub-nav loses the Knowledge entry mid-redeploy | Removing from `adminNavItems` is a single-file edit; no transitional period because the IconRail entry replaces it in the same PR. |
| Permission gate divergence | The IconRail entry uses `userStore.can('knowledge:read')`. Backend perms are unchanged. The gate matches what every endpoint already enforces server-side. |
| Existing tests reference `/admin/knowledge` paths | **None found.** No Cypress specs reference the path. The frontend has no unit test coverage of the KB UI; the only KB tests are the Go manager tests + frontend `slugify` / `bodyConvert` unit tests, neither of which references routes. |
| `formatDistanceToNow` locale handling | `date-fns` accepts a `locale` option. We don't currently pass app locale to other `date-fns` formatters either ([dataTableColumns.js](frontend/apps/main/src/features/admin/knowledge/dataTableColumns.js) uses `format` with no locale option). Stay consistent — no locale option in v1. Surface as open question for future. |
| Token-safety regression elsewhere | The `bg-green-100`/`bg-gray-100` violation in current ArticleForm is fixed by switching to `<Badge>`. No other token violations introduced. |

---

## 7. Decisions locked + remaining open questions

### Decided in this round (no longer open)
- Status pill colors: **existing Badge `default`/`secondary` variants**, no new variant.
- Filter-bar search: **server-side only** — DataTable's `:searchable="false"`, our debounced `<Input>` re-fetches with `search=`.
- Empty state: **DataTable's built-in** `<Ghost>` + `emptyText` covers both no-articles-yet and no-filter-matches.
- Token fix in `ArticleForm.vue` (`bg-green-100/text-green-800`): **yes**, fixed in this PR via `<Badge>`.

### Still open (non-blocking — confirm or defer)
1. **`formatDistanceToNow` locale.** App locale isn't threaded into date-fns formatters anywhere today. Plan ships without locale option in v1, matching existing precedent. Defer to a future i18n-date-polish pass unless you want it added now.

2. **Right-click on folder rows.** Existing `FolderTreeNode.vue` is hover-`⋮` only. Brief mentioned right-click as an alternative; plan stays hover-only (matches codebase, better a11y). Flagging for record — speak up if you want both.

3. **Sidebar "New folder" button position.** Locked to bottom of aside. Implementation needs `<aside class="flex flex-col">` with the tree in `flex-1 overflow-auto` and the button outside that flex item. Visual call — flagging if you want the button at the top of the tree instead.

4. **`<Knowledge.vue>` lint warning.** Single-word component name triggers `vue/multi-word-component-names`. Existing `Macros.vue`/`Tags.vue` carry the same warning — accepted precedent. Plan keeps the single-word name to match neighbours.

---

## Verification (post-implementation)

End-to-end smoke:
1. `pnpm dev:main` → click the new Knowledge icon on the rail → lands on `/knowledge` with the new layout.
2. Fold sidebar collapse/expand still works (PageHeader sidebar-toggle button).
3. Filter bar: type in Search → backend `ILIKE` results render; pick a Status → re-fetch; pick a Locale → re-fetch; reset → all articles return.
4. Click "New article" → form page; type a title → slug auto-fills; pick folder → breadcrumb updates from 2-segment to 3-segment.
5. Save → toast → land on `/knowledge/<id>/edit`; status badge "Draft"; click Publish in right sidebar → badge flips to Published.
6. Per-row dropdown on the list: Edit/Publish/Unpublish/Move/Delete all still work.
7. Visit a stale `/admin/knowledge` URL in the address bar → redirected to `/knowledge`.
8. `userStore.can('knowledge:read') === false` for a contact-only role → IconRail entry hidden.
9. Run `pnpm test` → existing `slugify.test.js` + `bodyConvert.test.js` pass.
10. `pnpm exec vite build --mode main` → build succeeds.

Backend:
- `go vet ./...` → clean (no backend changes).
- `go test ./internal/knowledge/...` → tests still skip cleanly without `DATABASE_URL`.

---

**Final deliverable path:** `docs/claude-code/b3-kb-ui-redesign-plan.md` (saved on plan-mode exit).
