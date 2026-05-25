# minidon — Coding Agent Guide (`AGENTS.md`)

This guide is compiled for AI coding agents to help you quickly understand the current state of the codebase, build and test mechanics, and how to implement the missing components.

---

## 1. Project Overview & Architecture

`minidon` is a Mastodon public-timeline viewer featuring real-time streaming updates and full-text search. It is a single-binary deployment: a Go backend embeds a compiled React/TypeScript single-page application (SPA).

### High-Level Component Relationship

```
                     [ Mastodon Instance ]
                              │
                              │ WebSocket public stream
                              ▼
                 [ internal/mastodon Client ]
                              │
                              │ chan *model.Status
                              ▼
                 [ internal/ingest Pipeline ]
                 /            │             \
  (sync write)  /             │ (debounced)  \ (concurrent fan-out)
               ▼              ▼               ▼
     [ internal/buffer ]  [ internal/index ]  [ SSE Subscribers ]
        Ring Buffer          MeiliSearch        GET /api/stream
               ▲              ▲                       ▲
               │              │                       │
       GET /api/timeline  GET /api/search             │
               ▲              ▲                       │
               └─────── [ internal/api ] ─────────────┘
                              ▲
                              │ HTTP
                      [ React SPA (web) ]
```

---

## 2. Implementation Gap Analysis (Planned vs. Built)

The detailed status of what has been implemented and what remains to be built can be found in [progress.md](docs/progress.md). Refer to that document to check current implementation gaps.


---

## 3. Compilation & Testing Instructions

The project uses a `Makefile` to manage building and testing. The Go build and test targets automatically depend on the compiled frontend assets (`web/dist/index.html`).

### Automatic Frontend Compilation
If you run `make build` or `make test` on a clean checkout, the `Makefile` will automatically trigger `make web` (compiling the React SPA via npm) before compiling/testing the Go code. This ensures `go:embed` compiles successfully without requiring manual frontend builds first.

Once compiled, subsequent Go builds/tests are fast as the frontend build step is skipped unless the build directory is cleaned.

### Build and Test Commands
You can build and test the project using the following `make` targets:

* **Run all Go tests** (automatically compiles frontend if missing):
  ```sh
  make test
  ```
* **Build the final self-contained Go binary** (automatically compiles frontend if missing):
  ```sh
  make build
  ```
* **Build frontend assets only**:
  ```sh
  make web
  ```
* **Build and run the application** (must define `MINIDON_MASTODON_INSTANCE` and `MINIDON_MASTODON_ACCESS_TOKEN` via environment variables or command-line flags):
  Using environment variables:
  ```sh
  MINIDON_MASTODON_INSTANCE=mastodon.social MINIDON_MASTODON_ACCESS_TOKEN=your-token make run
  ```
  Or directly using command-line flags on the compiled binary:
  ```sh
  ./bin/minidon --mastodon-instance=mastodon.social --mastodon-access-token=your-token
  ```
  To run the CLI subcommand instead:
  ```sh
  ./bin/minidon cli --format=text --mastodon-instance=mastodon.social --mastodon-access-token=your-token
  ```
* **Clean build artifacts** (removes built binary and compiled frontend assets):
  ```sh
  make clean
  ```


---

## 4. Implementation Details & Guidelines

Follow these specifications for implementing each component to ensure your code matches the architect's design goals.

### A. Ring Buffer (`internal/buffer`)
* **Goal**: Bounded, in-memory, thread-safe cache.
* **Interface**:
  ```go
  type Buffer struct {
      mu       sync.Mutex
      capacity int
      statuses []*model.Status
      // For lock-free reads, maintain an atomic snapshot pointer:
      snapshot atomic.Pointer[[]*model.Status]
  }
  func New(size int) *Buffer
  func (b *Buffer) Add(s *model.Status)
  func (b *Buffer) Recent(n int) []*model.Status
  ```
* **Concurrency**: Ensure writes to `statuses` are protected by `mu`. After updating the slice, package a new slice snapshot (reversing it or maintaining it chronologically as desired, but note that `Recent(n)` must return statuses in *reverse chronological order*) and store it in `snapshot` using `snapshot.Store()`. `Recent(n)` should then read from the snapshot without locking.

### B. Index / MeiliSearch (`internal/index`)
* **Goal**: Standard interface for full-text search backend.
* **Interfaces & Structs**:
  ```go
  package index

  import (
      "context"
      "github.com/amorken/minidon/internal/model"
  )

  type SearchOptions struct {
      Limit  int
      Offset int
  }

  type SearchResult struct {
      Hits   []model.Status `json:"hits"`
      Total  int64          `json:"total"`
      Limit  int            `json:"limit"`
      Offset int            `json:"offset"`
  }

  type Index interface {
      Index(statuses []model.Status) error
      Search(ctx context.Context, query string, opts SearchOptions) (SearchResult, error)
  }
  ```
* **MeiliSearch Details**:
  * Set index name to `"statuses"`.
  * Implement `meiliIndex` wrapping the `github.com/meilisearch/meilisearch-go` SDK.
  * Provide `EnsureSettings(ctx context.Context) error` to configure settings idempotently:
    * Searchable: `content`, `account.acct`, `account.display_name`, `tags.name`.
    * Sortable: `created_at`.
    * Filterable: `language`, `tags.name`.

### C. Ingest Pipeline (`internal/ingest`)
* **Goal**: Fan-out stream ingestion.
* **Design**:
  ```go
  type Pipeline struct {
      src          <-chan *model.Status
      buffer       *buffer.Buffer
      idx          index.Index
      
      mu           sync.RWMutex
      subscribers  map[chan *model.Status]struct{}
  }
  ```
* **Requirements**:
  * **Ring Buffer**: Immediate, synchronous write.
  * **MeiliSearch Index**: Implement a debounced batcher. Collect statuses and flush them when either **1 second has elapsed** OR **100 documents are queued**, whichever comes first. Use a ticker and a queue.
  * **SSE Fan-Out**: Maintain a set of active channels. Provide `Subscribe()` and `Unsubscribe(ch)` methods. On incoming status, loop over subscribers and write to their channels. Use non-blocking writes (`select { case ch <- status: default: // drop slow client }`) to prevent slow clients from blocking the ingest loop.

### D. HTTP API Handlers (`internal/api`)
* **Goal**: Mount logic on route patterns using method+pattern syntax (e.g., `"GET /api/timeline"`).
* **Routes**:
  * `GET /api/timeline?limit=N`: Fetch recent items from buffer (validate limit: default 50, max 200).
  * `GET /api/search?q=query&limit=20&offset=0`: Call `idx.Search(...)` and serialize `SearchResult` to JSON.
  * `GET /api/stream`:
    * Upgrade connection to SSE by setting:
      ```go
      w.Header().Set("Content-Type", "text/event-stream")
      w.Header().Set("Cache-Control", "no-cache")
      w.Header().Set("Connection", "keep-alive")
      ```
    * Subscribe to the Ingest Pipeline.
    * In a select loop, read statuses from subscription channel and write to client:
      ```go
      fmt.Fprintf(w, "event: status\ndata: %s\n\n", jsonData)
      flusher.Flush()
      ```
    * Unsubscribe when request context is cancelled (`<-r.Context().Done()`).
  * `GET /readyz`: Check if the Mastodon streaming client is connected before returning `200 OK` (else `503 Service Unavailable`).

### E. Wiring in `main.go`
* Parse the configuration using the `kong` library. Kong maps command-line arguments, subcommands (`web` and `cli`), and environment variables directly into the `config.Config` struct.
* Setup structured logging using `log/slog` (redirected to `os.Stderr` when running the `cli` subcommand to keep stdout clean, and configured for debug log level if the `--verbose` flag is passed).
* Define the execution path depending on the subcommand:
  * **CLI Mode (`cli` subcommand)**:
    * Validate Mastodon credentials.
    * Initialize the Mastodon client and stream statuses.
    * Print statuses to `stdout` in the selected `--format` (`json` or `text`).
    * Stop gracefully upon receiving `SIGINT` / `SIGTERM`.
  * **Web Mode (`web` subcommand or default)**:
    * Optionally warn if Mastodon configuration is missing, falling back to a `FakeClient` to generate mock data.
    * Instantiate the thread-safe bounded `buffer.Buffer` and `index.Index` (MeiliSearch index or no-op search index).
    * Call `index.EnsureSettings()` on startup.
    * Initialize the `mastodon.Client`.
    * Instantiate the `ingest.Pipeline` passing the client's output channel.
    * Start the ingest pipeline and connect the Mastodon stream in their respective goroutines.
    * Construct the router using `api.NewRouter(...)` and pass references to the buffer, index, and pipeline.
    * Start the `http.Server`. On shutdown, gracefully stop the HTTP server first, then the ingest pipeline, and finally close the Mastodon client.

### F. Frontend SPA (`web/src/App.tsx`)
* Replace the placeholder with a responsive layout.
* Maintain a React state/context array of statuses.
* On mount, fetch `/api/timeline` to seed the initial statuses.
* Establish an `EventSource('/api/stream')`. When a `"status"` event is received, prepend the new status to the state (maintaining timeline size at or below the view threshold).
* Add a search bar. Debounce user keystrokes (e.g. 300ms) and fetch search results from `/api/search?q=...`, then switch the view list to the search results.
* Ensure a clean layout, proper error boundaries, and connection state visualizer (e.g., connected vs disconnected from SSE).

---

## 5. Development Command Reference

Use the following commands during implementation and verification:

```sh
# 1. Start development services (MeiliSearch)
docker compose -f deploy/docker-compose.yml up meilisearch

# 2. Run developer environment (Vite HMR + Go server proxy)
./scripts/dev.sh

# 3. Running tests
make test

# 4. Linting Go code
make lint
```

---

## 6. Coding & Collaboration Guidelines

To maintain project quality and velocity, all developers and coding agents must follow these software engineering standards:

### A. Clean, Readable, and Maintainable Code
* **Readability Over Cleverness**: Code is read much more often than it is written. Prioritize clarity and simplicity. Avoid complex, single-line shortcuts when explicit blocks are easier to debug.
* **Consistency**: Follow the existing structures and conventions in the codebase. Maintain established naming schemes, package divisions, error handling styles, and formatting configurations.
* **Error Handling**: Always inspect returned errors. Wrap errors with meaningful context (e.g. `fmt.Errorf("ingest: failed to index batch: %w", err)`) rather than ignoring them or returning them naked.

### B. Commenting & Documentation
* **Explain the "Why", Not the "What"**: Code comments should explain the reasoning behind complex logic, design choices, concurrency conditions, or non-obvious constraints.
* **Package-Level Comments**: Keep package-level documentation comments up to date at the top of each Go file. Explain the high-level purpose of the package and its relationship to other components.
* **Concurrency Documentation**: Since this project deals with multi-threaded ingestion, clearly document locking assumptions (which mutex protects which fields) and channel ownership (which goroutine reads/writes/closes).

### C. Git Hygiene & Commit Messages
* **Atomic Commits**: Keep commits small, focused, and self-contained. A single commit should cover one logical unit of work (e.g. implementing the ring buffer, or adding a search query validation).
* **Descriptive Commit Messages**: Use structured, conventional commit messages. Format: `<type>(<scope>): <short description>` (e.g. `feat(buffer): implement lock-free concurrent reads via atomic snapshot pointer` or `test(static): verify cache control headers for static files`).
* **Clean Working Tree**: Ensure built binaries (`bin/`), temporary debug files, and local credentials are kept out of Git via `.gitignore`.

### D. Documentation Maintenance
* **Sync Changes with Docs**: Documentation under [docs/](docs/) must remain a source of truth.
* **Prevent Staleness**: If you add/modify environment configurations, HTTP API endpoints, data models, or design layouts:
  1. Update [docs/architecture.md](docs/architecture.md)
  2. Update [docs/api.md](docs/api.md)
  3. Update [docs/progress.md](docs/progress.md)
  Include documentation updates in the same commit or pull request as the code implementation.
