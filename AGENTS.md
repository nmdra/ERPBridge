# ERPBridge: Agent Integration & Development

This repository is an ERP middleware exposing functionality via the **Model Context Protocol (MCP)**. 

**Branching**: If you are connecting to this server to _use_ its tools, read **Usage**. If you are modifying this codebase, read **Development**.

---

## Usage (Consuming Tools)

Agents acting as MCP clients can consume ERPBridge via two transport layers. 

### Endpoints
- **Stdio**: Run `erpbridge-server --stdio`. (Use for local agents).
- **HTTP**: Connect to `http://localhost:8080/mcp/`. (Use for remote agents/Postman).

### Execution
1. **Discover**: Standard MCP `initialize` and `tools/list` lifecycle.
2. **Invoke**: Send tool calls. ERPBridge handles downstream ERP routing, resilience, and caching automatically.

_See [`docs/connectivity.md`](./docs/connectivity.md) for full session and auth configuration._

---

## Development (Modifying Code)

### Goals
- **Security**: Never leak downstream ERP credentials.
- **Resilience**: Handle upstream timeouts and errors gracefully.
- **Protocol Adherence**: Strictly follow the MCP specification.

### 1. Planning (Mandatory)
- **Check**: Read `.agents/plans/README.md`, then inspect `active/`, `upcoming/`, and `stalled/` before modifying code. Ignore `completed/` unless historical context is required.
- **Create**: If no plan covers your task, use the `plan` skill or `/plan` slash command to create one in `.agents/plans/upcoming/`.
- **Promote**: Move an approved plan to `.agents/plans/active/` before execution.
- **Execute**: Complete tasks sequentially. A task is done only when its `Verify:` command is green.
- **Close**: Prefix completed plan filenames with `[COMPLETED]` so other agents ignore them.

### 2. Commits
- **Atomic**: One plan task = one commit. 
- **Format**: Use Conventional Commits (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`, `test:`).
- **Exclude**: Never commit generated schemas or binaries unless explicitly planned.

### 3. Testing (TDD)
- **Workflow**: Red (write failing test) → Green (implement minimum code) → Refactor.
- **Location**: Place `*_test.go` beside the code it covers.
- **Patterns**: Use `httptest` servers for HTTP, `miniredis` for Redis, and `:memory:` SQLite for stores. 
- **Coverage**: Every behavior change requires a test.

### 4. Quality Gates
- **Fast Lint**: Only run `make lint` or `golangci-lint` on directories containing your changes. Do not run global lint sweeps unless tasked.
- **Pre-commit**: Run `make test` before finishing. Lefthook enforces this on commit.
- **Docs Sync**: Behavior changes must update the relevant `docs/` guide and `CHANGELOG.md` (Unreleased) in the same commit.

### 5. Secrets
- **Environment or Mounted File Only**: Resolve downstream ERP and plugin credentials via environment variables or an explicit per-resource `credentialSource: file` reference under `ERPBRIDGE_CREDENTIALS_DIR`; keep `API_AUTH_TOKEN` in the environment and never hard-code credential values.
- **Redaction**: Use `logger.RedactArgs` and the `masq` redaction layer to keep secrets out of logs and context.

### 6. Public Documentation
- **Repo**: [nmdra/erpbridge-docs](https://github.com/nmdra/erpbridge-docs) (local path: `~/Documents/Projects/erpbridge-docs`).
- **Truth**: In-repo `docs/` is the developer source of truth. The Docusaurus site is the user-facing truth. Keep them synced.
- **Action**: When adding features, updating CLI, or changing APIs, open a corresponding commit in the `erpbridge-docs` repository to reflect the change.
