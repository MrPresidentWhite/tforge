## TForge – AI Contributor Guide

This agent (AI assistant) acts as a **long‑term technical contributor** for this project.
It operates only on behalf of the maintainer and follows the rules below.

### General Scope

- Focus on:
  - **Code quality** (clean Go and frontend/TypeScript code)
  - **Security & secrets** (especially storage/protector/vault logic)
  - **Developer experience** (CLI flows, agent, Wails app)
- Use the **language configured for the agent/system** when communicating with users
  (e.g. German for this maintainer), but keep this file and code comments in **English**.
- Do not change project license, ownership information or legally relevant texts
  without explicit instruction.

### Git, Commits & Branches

- **Never change Git configuration** (no `git config`, no hook manipulation).
- Only create commits **when explicitly requested** by the user.
- Commit messages:
  - Short, descriptive English titles, e.g.:
    - `Use OS-backed DPAPI protector on Windows`
    - `Fix vault loading on Windows`
  - When possible, link issues via:
    - `Fixes #5`
  - Body: short bullet list explaining the *why* and the most important *what*.
- **Never push** unless the user explicitly asks for it.
- Whenever an AI agent contributes to a change, add it (and other involved agents)
  as **co-authors** in the commit message using Git's `Co-authored-by:` trailers.

### Issues & Milestones

- When an issue is mentioned in context (e.g. `#5`):
  - Prefer `Fixes #<nr>` / `Closes #<nr>` in commits if the change really solves it.
  - Keep README/Roadmap in sync (checkboxes, strikethrough, etc.) as requested
    by the maintainer.
- Milestones:
  - Link primarily through issues/PRs (not commits).
  - Use `gh` for PRs, labels, etc. only when the user explicitly wants that.

### Coding Standards (Backend, Go)

- Respect Go version and modules from `go.mod`.
- Before adding new dependencies, check whether existing packages/structure can be reused.
- Add new dependencies only when there is a clear benefit and the user is fine with it.
- Security‑sensitive areas:
  - `internal/secure/*` (protector, DPAPI, storage) must be treated with extra care.
  - No debug logs that leak secrets or key material.
  - Migrations of encryption formats must always:
    - keep backups
    - be clearly documented in the README
    - avoid silent data loss

### Coding Standards (Frontend)

- Wails/frontend:
  - Modern, readable UI, but no unnecessary experiments without being asked.
  - Respect existing build pipelines (`frontend/package.json`, Wails config).

### Tooling Usage

- **Go tooling**:
  - Run `go build ./...` (and, when appropriate, `go test ./...`) after substantial changes.
  - Use lints (`ReadLints`) after edits and fix newly introduced issues when possible.
- **gh CLI**:
  - Use `gh` only when the user explicitly requests GitHub operations
    (creating PRs, querying issues, etc.).
  - Never change repository settings.

### Security & Secrets

- Never invent or store real secrets, tokens or keys.
- For examples, always use **clearly fake** values.
- Do not create automatic backups or exports of `vaults.bin` / `master.key`
  outside of flows explicitly requested by the user.

### Communication with the Maintainer

- Explanations should be **concise and technically precise**, using the language
  configured for the agent/system.
- Always provide a short summary of changes (what / why), not the full diff.
- When things are ambiguous:
  - state assumptions explicitly,
  - make a reasonable default decision,
  - implement it so it is easy to undo or extend later.

