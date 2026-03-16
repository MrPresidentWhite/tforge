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
  - on **macOS/Linux**, `NewDefaultProtector` first tries a keyring‑backed
    `KeyringProtector` (Keychain / Secret Service via the OS keyring) and
    falls back to `SoftwareProtector` with a local AES‑256 key stored in the
    config directory if the keyring is not available
  - `Seal()` / `Unseal()` encrypt/decrypt the JSON‑encoded vault list

The Wails app and the `tforge-agent` both use the same storage layer and call
`NewDefaultProtector`, so they always see the same encrypted vault state for
the current user.

### Protector implementations & platform caveats

- **DPAPIProtector (Windows)**  
  - Uses the Windows **Data Protection API (DPAPI)** and is bound to the current user profile.
  - Encrypted vault data can only be decrypted under the same Windows account.
  - This avoids having to manage a separate key file, but you still need to protect your
    Windows account (login password, disk encryption, etc.).

- **KeyringProtector (macOS / Linux)**  
  - Uses the OS keyring (e.g. **Keychain** on macOS, **Secret Service**/**gnome‑keyring** on Linux)
    to store a randomly generated AES‑256 key.
  - Requires a working desktop keyring; on minimal/headless Linux systems without a keyring
    implementation, the keyring lookup will fail and TForge falls back to `SoftwareProtector`.
  - When the keyring is available, the raw AES key is never stored in plaintext on disk.

- **SoftwareProtector (all platforms, fallback)**  
  - Stores a random AES‑256 key in `master.key` under the config directory and uses it to
    encrypt `vaults.bin`.
  - This is the simplest and most portable option, but you are responsible for backing up
    `master.key` if you want to move vaults between machines, and you must protect access
    to the config directory itself.

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

GET /status
  -> 200 OK, JSON: { "locked": true|false, "timeoutSeconds": <int> }

POST /reload
  -> 200 OK, body: "reloaded"
```

- `POST /reload` re-reads `vaults.bin` and updates the in-memory vault list.
  The CLI calls this automatically after `--create-vault` so a running agent
  sees the new vault without restart. You can also call it manually after
  editing vaults on disk.

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

- Any request to `/health`, `/env`, `/lock`, `/unlock` or `/reload` counts as
  activity and resets the timer.
- If `--lock-timeout` is not set or is `0`, the inactivity timeout is
  disabled and the agent will not auto-lock.

In practice:

- The **lock state only affects API access** – it does not change how `vaults.bin` is
  encrypted at rest; that is entirely handled by the configured `Protector`.
- On a shared machine, always combine TForge with OS‑level protections (user accounts,
  full disk encryption, screen lock) and do not rely on the lock feature as the only
  security layer.

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

# import mode (create a new vault from an env-style file)
tforge --create-vault MyVault --file path/to/.env --type secrets --duplicate-to prod
```

Rules:

- positional order:
  - first non‑flag argument: vault reference (`@Name` or `ID`)
  - everything after that: command to run (optionally with a `--` separator)
- CLI calls the agent at `http://127.0.0.1:5959/env?...` and merges returned
  env vars into the child process’s `Env`.

Import mode:

- `tforge --create-vault <Name> --file <path>` creates a new vault directly
  in the local storage, importing keys from an env-style file (`KEY=VALUE`,
  `#` comments supported).
- If the agent is running, the CLI triggers a reload so the new vault is
  visible immediately; otherwise restart the agent to see it.
- Values are imported into the `dev` environment by default; use
  `--duplicate-to staging` or `--duplicate-to prod` to copy the same values
  into another environment.
- `--type` controls the entry type (`secrets` – default, `env`, or `note`).

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

## Installation on Linux and macOS (CLI + Agent)

TForge is intended to work on modern Linux distributions and recent macOS
versions with:

- a recent Go toolchain,
- **systemd user services** on Linux (for convenient autostart),
- or **LaunchAgents** on macOS,
- and a desktop keyring implementation (for the keyring‑backed protector).

### Quick install via script (recommended)

You can install the CLI and agent into a user bin directory and set up
autostart for `tforge-agent` with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/MrPresidentWhite/tforge/main/install-tforge-tools.sh | bash
```

You can also override the install directory (for example `~/.tforge/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/MrPresidentWhite/tforge/main/install-tforge-tools.sh | bash -s -- "$HOME/.tforge/bin"
```

The script will:

- build `tforge` and `tforge-agent`,
- place them into the chosen directory,
- ensure that directory is on your `PATH` (by updating `~/.profile` if needed),
- and, depending on the platform:
  - on **Linux**, if `systemd --user` is available, create and enable a
    `tforge-agent.service` user unit that starts the agent automatically
    on login,
  - on **macOS**, create a `~/Library/LaunchAgents/dev.tforge.agent.plist`
    LaunchAgent that runs `tforge-agent` on login and keeps it alive.

### Manual build (from a local clone)

From the repo root:

```bash
go build ./cmd/tforge-agent
go build ./cmd/tforge
```

Place the resulting binaries somewhere on your `PATH`, for example `~/.local/bin`.

### Optional: systemd user service for autostart

Create a user service unit at `~/.config/systemd/user/tforge-agent.service`:

```ini
[Unit]
Description=TForge local vault agent

[Service]
ExecStart=%h/.local/bin/tforge-agent
Restart=on-failure

[Install]
WantedBy=default.target
```

Reload and enable the service:

```bash
systemctl --user daemon-reload
systemctl --user enable --now tforge-agent.service
```

This starts `tforge-agent` automatically for your user session on login. You can
inspect the status and logs via:

```bash
systemctl --user status tforge-agent.service
journalctl --user -u tforge-agent.service
```

---

## Roadmap / Ideas

**v1 – Core security & platform support**

- [x] ~~OS‑backed `Protector` on Windows (DPAPI)~~
- [x] ~~OS‑backed `Protector` on macOS/Linux (Keychain / Secret Service)~~
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

