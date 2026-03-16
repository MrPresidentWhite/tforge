# TForge – Local Secrets & Vault Runner

TForge is a **local‑first vault and secret management tool** for application developers.
It lets you:

- define environment variables and secrets per vault and per environment (`dev`, `staging`, `prod`)
- store them **encrypted at rest** on your machine
- inject them into any local command (e.g. `npm run dev`, `docker compose up`, etc.) **without ever writing a `.env` file in plaintext**

The project is heavily inspired by [`keypo-cli`](https://github.com/keypo-us/keypo-cli),
specifically the `vault exec` workflow in `keypo-signer`, and adapts similar ideas to a
cross‑platform Go + Wails desktop app.

> **Credits**  
> Huge thanks to the authors of [`keypo-cli`](https://github.com/keypo-us/keypo-cli) for the
> original design and inspiration. TForge deliberately reuses the idea of “run a command with
> secrets injected as environment variables, without ever touching a `.env` file”.

---

## High‑level Architecture

TForge consists of three main pieces:

- **Wails Desktop App** (`wails dev` / `wails build`)
  - UI to create vaults, group keys, manage per‑environment values (`dev`, `staging`, `prod`)
  - state is kept in memory via `internal/vault.Service`
  - changes are persisted in encrypted form via `internal/storage` + `internal/secure`

- **Local Agent / Daemon** (`cmd/tforge-agent`)
  - small HTTP server bound to `127.0.0.1:5959`
  - loads the same encrypted vault file as the GUI and exposes a minimal RPC API:
    - `GET /health`
    - `GET /env?vault=<nameOrID>&env=<dev|staging|prod>`
  - returns a JSON map `{ "env": { KEY: VALUE, ... } }` for the requested vault and environment

- **CLI Runner** (`cmd/tforge`)
  - command‑line front‑end that talks to the agent and starts arbitrary processes with injected env vars
  - two main modes:
    - **exec mode**: start a process with env from a vault
    - **export mode**: print `KEY=VALUE` lines for integration with other tooling

The storage format is **local‑only**, there is no cloud backend.

> **Status / Security disclaimer**
>
> TForge is currently a **personal proof‑of‑concept** and **not a production‑ready tool**.
> The security model and implementation have **not been professionally reviewed or audited**.
> Do **not** use it to store highly sensitive secrets in environments with strict security
> or compliance requirements.

---

## Storage & Encryption

Persistent state lives under the OS config directory, e.g. on Windows:

- `ConfigDir()` → `%APPDATA%\TForge`
- vault data file: `vaults.bin`

The code paths are:

- `internal/storage/vault_store.go`
  - `ConfigDir()` – base config path
  - `LoadVaults()` / `SaveVaults()` – read/write encrypted blob

- `internal/secure/protector.go` + platform‑specific defaults in
  `internal/secure/protector_default*.go`
  - `Protector` is the abstraction used by the app and agent
  - on **Windows**, `NewDefaultProtector` uses a `DPAPIProtector` (Windows DPAPI,
    bound to the current user account) and falls back to `SoftwareProtector` only
    if DPAPI is not available
  - on non‑Windows platforms, `NewDefaultProtector` currently uses `SoftwareProtector`
    with a local AES‑256 key stored in the config directory
  - `Seal()` / `Unseal()` encrypt/decrypt the JSON‑encoded vault list

The Wails app and the `tforge-agent` both use the same storage layer and call
`NewDefaultProtector`, so they always see the same encrypted vault state for
the current user.

> **Migration note**  
> Older versions used only `SoftwareProtector` with a local `master.key` file.
> Current builds on Windows use DPAPI by default; existing installations should
> migrate their vaults before dropping legacy artefacts or switching machines.

---

## Vault Model

The core data model (`internal/vault/vault.go`):

- `Vault`
  - `ID`, `Name`, optional `Icon`, `Description`
  - `Entries []Entry`

- `Entry`
  - `Key` – final environment variable name (e.g. `POSTGRES_HOST`)
  - `ValueDev`, `ValueStage`, `ValueProd` – per‑environment values
  - `Type` – `"env"`, `"secret"`, `"note"` (UI uses this to style entries)
  - `GroupPrefix` – optional grouping prefix (e.g. `POSTGRES_`) for nicer UI

The Wails frontend allows:

- grouping keys via `GroupPrefix` (e.g. `POSTGRES_` → `POSTGRES_HOST`, `POSTGRES_USER`, …)
- editing values for `dev`, `staging`, `prod`
- bulk adding keys within a group
- duplicating `dev` values into `staging` or `prod` via the custom context menu

---

## Local Agent (`tforge-agent`)

Located in `cmd/tforge-agent/main.go`.

Responsibilities:

- initialize `secure.Protector` and `vault.Service`
- load `vaults.bin` on startup via `storage.LoadVaults`
- serve a tiny HTTP API on `127.0.0.1:5959`:

```http
GET /health
  -> 200 OK, body: "ok"

GET /env?vault=<nameOrID>&env=<dev|staging|prod>
  -> 200 OK, JSON: { "env": { KEY: VALUE, ... } }
  -> 404 if vault not found

POST /lock
  -> 200 OK, body: "locked"

POST /unlock
  -> 200 OK, body: "unlocked"
```

Lock semantics:

- When the agent is **locked**, `/env` refuses to return any environment
  data and instead responds with `423 Locked` and a short error message
  (`"agent is locked; env access disabled"`).
- Lock/unlock is intentionally simple and **local-only** in this first
  iteration; there is no authentication yet. Future versions may add
  proper session-based security and re-auth flows.

Inactivity timeout:

- The agent supports an optional inactivity timeout that will **re-lock**
  the agent after a period without activity.
- It is configured via the `--lock-timeout` flag, for example:

  ```bash
  tforge-agent --lock-timeout=15m
  ```

- Any request to `/health`, `/env`, `/lock` or `/unlock` counts as
  activity and resets the timer.
- If `--lock-timeout` is not set or is `0`, the inactivity timeout is
  disabled and the agent will not auto-lock.

Vault lookup:

- matches either by `Vault.ID` or `Vault.Name`

Env mapping (when unlocked):

- picks `ValueDev` / `ValueStage` / `ValueProd` based on `env` query parameter
- skips empty values
- currently does not filter by `Entry.Type` (can be refined later)

The agent is designed to be simple and local‑only. There is no authentication yet
because the process is expected to run under the current user and only listen on
`127.0.0.1`. A future enhancement is to add an explicit unlock / re‑auth flow.

---

## CLI Runner (`tforge`)

Located in `cmd/tforge/main.go`.

### Usage

```bash
# default env = dev
tforge @MyVault npm run dev

# explicit env selection
tforge --env dev @MyVault npm run dev
tforge --env staging @MyVault npm run dev
tforge --env prod @MyVault npm run dev

# export mode (no process, just KEY=VALUE to stdout)
tforge --env dev --export @MyVault
```

Rules:

- positional order:
  - first non‑flag argument: vault reference (`@Name` or `ID`)
  - everything after that: command to run (optionally with a `--` separator)
- CLI calls the agent at `http://127.0.0.1:5959/env?...` and merges returned
  env vars into the child process’s `Env`.

Example:

```bash
# in one terminal
tforge-agent

# in another
cd my-project
tforge --env dev @MyVault npm run dev
```

No `.env` file is created on disk; the secrets live only in memory and in the
environment of the child process.

---

## Development

### Wails App (GUI)

```bash
cd tforge
wails dev      # live reloading UI + Go
wails build    # build production bundle
```

### Agent & CLI

```bash
# from repo root
go run ./cmd/tforge-agent
go run ./cmd/tforge --env dev @MyVault npm run dev
```

You can also use the helper script `install-tforge-tools.ps1` (Windows/PowerShell)
to build `tforge` and `tforge-agent` into a `~/.tforge/bin` directory, add
it to your PATH and (on Windows) set up auto‑start for the agent.

---

## Installation on Windows (CLI + Agent)

Requirements:

- Go installed and on `PATH`
- PowerShell (default on Windows 10+)

Steps:

1. Open a PowerShell window in the project root (`tforge`).
2. Run the install script:

   ```powershell
   ./install-tforge-tools.ps1
   ```

   - Default install path is `~\.tforge\bin`.
   - This directory is added to your user `PATH` so `tforge` and `tforge-agent`
     are available in new terminals.

3. Agent auto‑start:

   - The script creates a shortcut `tforge-agent.lnk` in your user Startup folder.
   - This makes `tforge-agent.exe` start automatically in the background when you sign in to Windows.
   - In normal use you do not need to start the agent manually.

4. Optional: disable auto‑start

   - Open  
     `%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup`
   - Delete the `tforge-agent.lnk` shortcut.

After that you can run, from any project directory:

```bash
tforge --env dev @MyVault npm run dev
```

---

## Roadmap / Ideas

**v1 – Core security & platform support**

- [x] ~~OS‑backed `Protector` on Windows (DPAPI)~~
- [ ] OS‑backed `Protector` on macOS/Linux (Keychain / Secret Service)
- [ ] improved agent security and unlock flows (session timeouts, re‑auth, optional PIN / biometrics)
- [ ] first‑class Linux support (packaging, autostart, desktop integration)
- [ ] clear headless/CI story for using vaults in build pipelines

**v2 – Developer experience & integrations**

- [ ] more granular export modes (e.g. filter by group or type)
- [ ] vault templates/presets for common stacks (e.g. Postgres + Redis + Next.js)
- [ ] deeper tooling integration (Docker Compose, kubectl, Terraform, IDE extensions)
- [ ] local audit / activity log for vault usage (without logging secret values)

**Later – Advanced features**

- [ ] vault sync across multiple TPM‑capable machines (secure, hardware‑backed)
- [ ] encrypted backup/export and restore flow for vaults (disaster recovery)
- [ ] optional `.env` generation for CI/CD only (not for local dev)

---

## Contributing

Contributions, feedback and ideas are very welcome.

- **Issues**: Open issues for bugs, feature requests or questions.
- **Pull requests**: Prefer small, focused PRs with a short description of the motivation and main changes.
- **Discussion**: For larger changes, open an issue first to discuss the design before you start implementing.

When using AI assistants or agents (for example to generate code or refactors),
please keep them aligned with the project-specific guidance in `AGENTS.md` and
the Cursor rule `.cursor/rules/ai-contributor.mdc` (commit behaviour, security
considerations and co-author attribution).

---

## License

This project is licensed under the MIT License. See [LICENSE](./LICENSE) for details.

