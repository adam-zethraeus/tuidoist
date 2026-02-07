# Migrate from REST API to Sync API

## Why

The current app makes individual REST calls per operation: paginated GET /projects,
GET /tasks?project_id=X per project, one POST per mutation. The Sync API replaces all
of this with a single POST /sync endpoint that returns all changed data incrementally
and accepts batched write commands.

**Benefits:**
- One sync call replaces N per-project fetches (instant project switching from cache)
- Batched mutations: send 10 pending commands in one request instead of 10 requests
- Incremental sync: only changed data transferred via sync_token (no TTL heuristics)
- Built-in idempotency: command UUIDs prevent double-execution on retry
- temp_id_mapping: server resolves pending IDs for creates automatically
- Eliminates background per-project refresh chains entirely

## Architecture Change

### Current Flow
```
App start → FetchProjects (paginated REST)
          → Per-project: FetchTasks (paginated REST)
          → Background: check TTL per project, chain-refresh stale ones
Mutations → one REST call per mutation, sequential flush
```

### Target Flow
```
App start → Load cache (instant) → IncrementalSync (one POST)
          → All projects/tasks/sections arrive in one response
          → Periodic sync every 30s replaces background refresh
Mutations → batch all pending commands in next sync request
```

## Sync API Reference

**Endpoint:** `POST https://api.todoist.com/api/v1/sync`
**Content-Type:** `application/x-www-form-urlencoded`
**Auth:** `Authorization: Bearer {token}`

**Read params:** `sync_token` (`*` for full, or previous token), `resource_types` (JSON array)
**Write params:** `commands` (JSON array of command objects)
**Response:** `{ sync_token, full_sync, projects[], items[], sections[], labels[], temp_id_mapping{}, sync_status{} }`

**Command format:**
```json
{ "type": "item_add", "uuid": "...", "temp_id": "...", "args": { "content": "...", "project_id": "..." } }
```

**Command types we use:**
| Current Action | Sync Command |
|---|---|
| CreateTask | `item_add` |
| UpdateTask | `item_update` |
| CloseTask | `item_complete` (args: `{ids: [...]}`) |
| ReopenTask | `item_uncomplete` (args: `{ids: [...]}`) |
| DeleteTask | `item_delete` (args: `{ids: [...]}`) |
| CreateProject | `project_add` |
| ArchiveProject | `project_archive` |
| UnarchiveProject | `project_unarchive` |

**Priority mapping note:** Sync API uses inverted priorities (1=normal, 4=urgent) vs REST
(1=urgent, 4=normal). Verify whether the unified API v1 sync endpoint follows REST or Sync
convention. May need a mapping layer.

## Implementation Plan

### Phase 1: sync.go — New Sync Client

New file alongside api.go. Does not replace api.go yet.

```go
type SyncClient struct {
    token string
    http  *http.Client
}

type SyncResponse struct {
    SyncToken    string                  `json:"sync_token"`
    FullSync     bool                    `json:"full_sync"`
    Projects     []Project               `json:"projects"`
    Items        []Task                  `json:"items"`
    Sections     []Section               `json:"sections"`
    Labels       []Label                 `json:"labels"`
    TempIDMap    map[string]string       `json:"temp_id_mapping"`
    SyncStatus   map[string]interface{}  `json:"sync_status"`
}

type SyncCommand struct {
    Type   string         `json:"type"`
    UUID   string         `json:"uuid"`
    TempID string         `json:"temp_id,omitempty"`
    Args   map[string]any `json:"args"`
}
```

Methods:
- `Sync(token string, resourceTypes []string) (*SyncResponse, error)`
- `SyncWithCommands(token string, resourceTypes []string, cmds []SyncCommand) (*SyncResponse, error)`

Both call the same endpoint; the second also sends `commands`.

### Phase 2: store.go — Sync Token + Incremental Merge

Replace TTL-based staleness with sync_token:

- Add `GetSyncToken() string` — reads from sync_meta
- Add `SetSyncToken(token string)` — writes to sync_meta
- Add `MergeSyncResponse(resp *SyncResponse) error`:
  - For each project: upsert if not is_deleted, delete if is_deleted
  - For each item: upsert if not is_deleted and not checked, delete if is_deleted or checked
  - For each section: upsert if not is_deleted, delete if is_deleted
  - Handle is_archived on projects (move to archived_projects table)
  - Save completed items to completed_tasks table
- Keep `IsStale` and `TouchSync` temporarily for backward compat during transition
- Keep all existing per-table read methods (GetProjects, GetTasks, etc.)

### Phase 3: repo.go — Replace Reads with Sync

Replace the multi-method fetch approach with a single sync method:

**Remove:**
- `FetchProjects`, `RefreshProjects`, `fetchProjectsFromAPI`
- `FetchTasks`, `RefreshTasks`, `fetchTasksFromAPI`
- `FetchSections`, `RefreshSections`, `fetchSectionsFromAPI`
- `FindStaleProjects`, `BackgroundRefreshProject`

**Add:**
- `PerformSync() tea.Cmd` — loads sync_token, calls SyncClient.Sync, merges response, returns `syncDoneMsg`
- `GetCachedTasks` / `GetCachedSections` / `GetCachedProjects` stay (read from SQLite)

New message type:
```go
type syncDoneMsg struct {
    projects []Project  // all current projects (from cache after merge)
    err      error
}
```

### Phase 4: repo.go — Batched Command Flush

Replace one-at-a-time flush with batched command flush:

**Remove:**
- `FlushNext` (sequential single-mutation flush)
- `flushCreate`, `flushUpdate`, `flushClose`, `flushDelete`, `flushReopen`
- `detectConflict`, `snapshotTask` (snapshot-based conflict detection)

**Add:**
- `FlushPending() tea.Cmd`:
  1. Gather all pending mutations from queue
  2. Convert each to a `SyncCommand` with a UUID
  3. Call `SyncClient.SyncWithCommands(token, resourceTypes, commands)`
  4. Process `sync_status`: mark "ok" commands as flushed, mark errors as conflicted
  5. Process `temp_id_mapping`: replace pending task/project IDs with real IDs in cache
  6. Merge incremental response into cache
  7. Return `flushDoneMsg`

**Mutation → Command conversion:**
```go
func mutationToCommand(m Mutation) SyncCommand {
    switch m.Action {
    case MutationCreate:
        var req createTaskRequest
        json.Unmarshal([]byte(m.Payload), &req)
        return SyncCommand{
            Type:   "item_add",
            UUID:   fmt.Sprintf("mut-%d", m.ID),
            TempID: m.EntityID, // "pending-xxx"
            Args:   map[string]any{"content": req.Content, "project_id": req.ProjectID, ...},
        }
    case MutationClose:
        return SyncCommand{
            Type: "item_complete",
            UUID: fmt.Sprintf("mut-%d", m.ID),
            Args: map[string]any{"ids": []string{m.EntityID}},
        }
    // ... etc
    }
}
```

**Keep:**
- Optimistic mutation methods (CloseTask, CreateTask, etc.) — they still write to cache
  immediately for instant UI, and enqueue to mutation_queue
- Mutation queue table (offline resilience)
- Queue view (still shows pending/conflicted mutations)

### Phase 5: app.go — Wire Sync into UI

**Replace message handling:**
- Remove handlers for: `cachedProjectsMsg`, `projectsMsg`, `cachedTasksMsg`, `tasksMsg`,
  `cachedSectionsMsg`, `sectionsMsg`, `backgroundRefreshMsg`, `backgroundRefreshDoneMsg`
- Add handler for `syncDoneMsg`:
  - Update projects list
  - If current project's tasks changed, reload task display from cache
  - Reset loading state

**Add periodic sync:**
```go
const syncInterval = 30 * time.Second

type syncTickMsg struct{}

// In Init():
tea.Batch(a.spinner.Tick, a.repo.PerformSync(), periodicSync())

// On syncDoneMsg:
cmds = append(cmds, periodicSync()) // restart timer

// On syncTickMsg:
return a, a.repo.PerformSync()
```

**Replace 'r' refresh:** Fire `PerformSync()` instead of per-resource refresh.

**Replace Init():**
```go
func (a App) Init() tea.Cmd {
    return tea.Batch(
        a.spinner.Tick,
        a.loadFromCache(),     // instant: populate views from SQLite
        a.repo.PerformSync(),  // async: incremental sync
        a.repo.FlushPending(), // flush any leftover mutations
    )
}
```

**tasks.go simplification:**
- `LoadProject` just reads from cache (no async fetch)
- Remove `loading` bool — cache is always populated after initial sync
- Keep `completedTasks` in-memory behavior (unchanged)

### Phase 6: Cleanup

- Remove REST API pagination methods from api.go (GetProjects, GetTasks, GetSections)
- Keep api.go for: QuickAdd (not available via sync), GetComments, GetLabels (if needed)
- Remove `PaginatedResponse` type
- Remove `IsStale`, `TouchSync` from store.go
- Remove `cachedProjectsMsg`, `cachedTasksMsg`, `cachedSectionsMsg` from types.go
- Remove `backgroundRefreshMsg`, `backgroundRefreshDoneMsg` from types.go
- Remove `Snapshot` field from Mutation struct (no longer needed for conflict detection)
- Remove `bgRefreshStarted` from App

## Files Changed

| File | Change |
|---|---|
| **sync.go** | NEW — SyncClient, SyncResponse, SyncCommand |
| **store.go** | Add sync_token storage, MergeSyncResponse; remove TTL |
| **repo.go** | Replace multi-fetch with PerformSync, FlushPending; remove snapshot conflicts |
| **types.go** | Add syncDoneMsg; remove cached*Msg, backgroundRefresh*Msg |
| **app.go** | Replace per-resource handlers with syncDoneMsg; add periodic sync timer |
| **tasks.go** | Simplify LoadProject (cache-only); remove two-phase message handlers |
| **projects.go** | Remove FetchProjects call from Init; handle syncDoneMsg |
| **api.go** | Remove pagination methods; keep QuickAdd, GetComments |
| **queue.go** | Adapt to new flush model (mostly unchanged) |
| **completed.go** | Unchanged |
| **theme.go** | Unchanged |
| **main.go** | Pass SyncClient to Repository |

## Verification

1. `go build` / `go vet` clean at each phase
2. Initial full sync populates all projects and tasks
3. Switching projects is instant (cache-only read)
4. Creating/completing/deleting tasks updates locally + flushes via sync commands
5. Periodic sync picks up changes from other Todoist clients
6. Pending mutations survive app restart (mutation_queue persisted)
7. Conflicted commands appear in queue view (Q)
8. temp_id_mapping correctly replaces "pending-xxx" IDs after create flush

## Risk: Priority Inversion

Sync API v9 uses inverted priorities (1=normal, 4=urgent) vs REST API (1=urgent, 4=normal).
The unified API v1 sync endpoint needs to be tested to determine which convention it follows.
If inverted, add a mapping layer in MergeSyncResponse and mutationToCommand.

## Risk: Large Initial Sync

Users with thousands of tasks will have a large initial sync response. Mitigate by:
- Showing cached data immediately while sync runs
- Using `resource_types=["projects","items","sections"]` (not "all")
- Persisting sync_token so subsequent launches are incremental
