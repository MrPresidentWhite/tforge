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
  - loads the same encrypted vault file as the GUI and exposes a minimal RPC API
    (`/health`, `/env`, `/lock`, `/unlock`, `/status`, `/reload`)
  - returns a JSON map `{ "env": { KEY: VALUE, ... } }` for the requested vault and environment
  - **starts locked**: `/env` stays closed until something unlocks it, which on
    supported platforms requires an OS re‑authentication step

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

If a protector cannot be initialised (DPAPI unavailable, no working keyring),
TForge falls back to `SoftwareProtector` and **logs that it did so**. The
fallback is a security downgrade — it writes a `master.key` next to the vault
instead of letting the OS hold the key — so it should never happen silently.

### On‑disk format

`vaults.bin` carries a small header in front of the sealed payload:

```
0..4  magic "TFVLT"
5     format version (currently 1)
6     which protector sealed the payload (software / DPAPI / keyring)
7..   sealed payload
```

Without it the file was a bare opaque blob, which meant a failed decrypt
could not be told apart from a corrupt file, and any later format change
would have been guesswork.

- **Files written before the header still load.** They start straight into the
  sealed payload; TForge detects that and falls back to a plain unseal. The
  next save writes the header, so a file upgrades itself with no migration
  step.
- **A newer format version is refused outright**, rather than being decrypted
  on a guess.
- **The header is advisory, not authenticated.** It sits outside the AEAD, so
  editing the protector byte can only produce a misleading error message — it
  can never make a payload decrypt that otherwise would not.

### Refusing to overwrite unreadable vault data

If `vaults.bin` exists but cannot be decrypted, the GUI does **not** start with
an empty vault list. It keeps the error, disables all persistence and shows a
warning banner. Without this, the next save would replace the encrypted file
with whatever is in memory (usually nothing) and destroy the vaults.

The most likely trigger is a protector mismatch: a legacy `master.key`
installation opened by a build that now uses DPAPI. Thanks to the header the
error says so explicitly — which protector sealed the file and which one is in
use now — instead of only reporting a failed authentication tag.
`App.StartupError()` exposes the reason to the frontend.

> **Migration note**  
> Older versions used only `SoftwareProtector` with a local `master.key` file.
> Current builds on Windows use DPAPI by default; existing installations should
> migrate their vaults before dropping legacy artefacts or switching machines.
> If you see the warning banner described above, your `vaults.bin` is intact —
> it is simply sealed with a different protector than the one currently in use.

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

The Wails frontend shows all three environments **side by side**, one column
each, so a missing `staging` value is visible without switching modes. Each
environment has its own colour (`dev` blue, `staging` amber, `prod` red), which
doubles as a warning cue when editing production values.

It allows:

- searching keys **and** values (`/` or `Ctrl+F`)
- grouping keys via `GroupPrefix` (e.g. `POSTGRES_` → `POSTGRES_HOST`, `POSTGRES_USER`, …),
  rendered as collapsible blocks
- editing any value inline — click a cell, `Tab` moves to the next environment
  in the same row, `Esc` discards
- bulk adding keys, either standalone or under a shared prefix
- selecting rows via checkboxes; the actions for the selection (group them,
  copy `dev` into `staging`/`prod`, delete) appear in a bar rather than being
  hidden behind a right‑click
- masking `secret` values, revealed per row or globally
- restricting the view to a single environment when the window is narrow

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
  -> 400 if the vault parameter is missing
  -> 404 if vault not found
  -> 423 Locked if the agent is locked

POST /lock
  -> 200 OK, body: "locked"

POST /unlock
  -> 200 OK, body: "unlocked"
  -> 401 if the OS re-authentication failed or was cancelled

GET /status
  -> 200 OK, JSON: { "locked": true|false, "timeoutSeconds": <int> }

POST /reload
  -> 200 OK, body: "reloaded"
  -> 404 if there is currently no vault file on disk
```

Every endpoint answers `405` for the wrong HTTP method, and `403` for requests
that did not come from a local, non-browser client (see below).

- `POST /reload` re-reads `vaults.bin` and updates the in-memory vault list.
  The CLI calls this automatically after `--create-vault` so a running agent
  sees the new vault without restart. You can also call it manually after
  editing vaults on disk. A missing vault file is reported as `404` rather than
  applied — otherwise a file that momentarily disappears would silently wipe
  every vault the agent still holds.

### Who may talk to the agent

Binding to loopback is not enough on its own: any web page the user visits can
send a request to `127.0.0.1` from their browser. Without a guard, a malicious
page could `POST /unlock` and pop a Windows Hello prompt, and a DNS rebinding
attack could go on to read `/env`.

The agent therefore rejects a request with `403` when:

- the `Host` header is not its own loopback address — a request addressed to
  some other name that merely resolves to `127.0.0.1` is a rebinding attempt, or
- the request carries an `Origin` or `Sec-Fetch-Site` header, which means a
  browser sent it. Nothing that legitimately talks to the agent runs in a browser.

This is a hardening measure, not an authentication scheme. Any local process
running as the same user can still reach the agent; the lock state and the
re‑authentication step below are what stand between such a process and the
secrets.

Lock semantics:

- When the agent is **locked**, `/env` refuses to return any environment
  data and instead responds with `423 Locked` and a short error message
  (`"agent is locked; env access disabled"`).
- The agent starts in a **locked** state by default. Unlocking requires
  a POST to `/unlock`, which may trigger a short OS-level re-authentication
  step (e.g. Windows Hello, macOS login / Touch ID) on supported platforms.

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
- currently does not filter by `Entry.Type`, so `note` entries are injected as
  environment variables like any other key (can be refined later)

### OS re‑authentication on unlock

`POST /unlock` calls `secure.RequireOSReauth()` before changing the lock state:

- **Windows** – runs `tforge-hello-helper.exe`, a small WinRT helper that asks
  `UserConsentVerifier` for Windows Hello. Exit code `0` means verified;
  anything else (including a cancelled prompt) fails the unlock. The call is
  bounded by a 60 s timeout so an unanswered dialog cannot block the agent.
- **macOS / Linux** – currently a stub that always succeeds. Since the agent
  starts locked, this means any local process on those platforms can unlock it.
  Touch ID / LocalAuthentication is still open work.

The helper is looked up **only** next to the agent executable, either directly
or in a `helper-bin/` subdirectory. It is deliberately never taken from the
working directory: the helper's exit code is what authorises unlocking, so
picking one up from wherever the agent happens to have been started would let
anyone who can write to that directory plant a binary that exits `0`.

For development — where `go run` places the agent in a temporary directory —
point `TFORGE_HELLO_HELPER` at the helper instead:

```powershell
$env:TFORGE_HELLO_HELPER = "C:\path\to\tforge\helper-bin\tforge-hello-helper.exe"
```

Building the agent into the repository root instead of using `go run` needs no
environment variable, because `helper-bin/` then sits next to the binary.

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

# export quoted for a shell
eval "$(tforge --env dev --export --export-format shell @MyVault)"

# import mode (create a new vault from an env-style file)
tforge --create-vault MyVault --file path/to/.env --type secrets --duplicate-to prod

# encrypted backup and restore
tforge --backup vaults.tfbak
tforge --restore vaults.tfbak

# delete a vault (note: flags must come before the vault reference)
tforge --delete -y @MyVault
```

Rules:

- positional order:
  - first non‑flag argument: vault reference (`@Name` or `ID`)
  - everything after that: command to run (optionally with a `--` separator)
- because of that, **flags have to come before the vault reference**. Go's flag
  package stops parsing at the first non‑flag argument, which is exactly what
  makes `tforge @MyVault npm run dev -- --port 3000` pass the trailing flags to
  the child process rather than to `tforge`.
- CLI calls the agent at `http://127.0.0.1:5959/env?...` and merges returned
  env vars into the child process’s `Env`.

Unlocking:

- Before fetching env data the CLI asks the agent for `/status` and only sends
  `/unlock` when the agent actually reports itself locked. That keeps the OS
  re‑authentication prompt to roughly once per session instead of once per
  command.

Export mode:

- Keys are always sorted, so repeated runs produce identical output. (Go
  randomises map iteration order, which previously made the output differ on
  every invocation.)
- `--export-format raw` (default) prints plain `KEY=VALUE` lines.
- `--export-format shell` single‑quotes each value so the output survives
  `eval`. Without quoting, a value containing a space, newline or semicolon
  would be split by the shell or, in the worst case, executed as a command of
  its own. Use this whenever the output is fed to a shell.

Import mode:

- `tforge --create-vault <Name> --file <path>` creates a new vault directly
  in the local storage, importing keys from an env-style file (`KEY=VALUE`,
  `#` comments supported).
- The parser also handles what real `.env` files usually contain: a leading
  `export ` on the key, values wrapped in matching single or double quotes, and
  surrounding whitespace around unquoted values. Whitespace *inside* quotes is
  preserved. When a key appears twice the last assignment wins, matching how
  shells and dotenv loaders behave.
- Vault names must be unique. The agent resolves a reference by ID *or* name
  and takes the first match, so a duplicate name would make `tforge @Name`
  ambiguous; the import refuses it instead.
- If the agent is running, the CLI triggers a reload so the new vault is
  visible immediately; otherwise restart the agent to see it.
- Values are imported into the `dev` environment by default; use
  `--duplicate-to staging` or `--duplicate-to prod` to copy the same values
  into another environment.
- `--type` controls the entry type (`secrets` – default, `env`, or `note`).

### Backup & restore

`vaults.bin` is sealed by a key that never leaves the machine: DPAPI is bound
to the Windows user profile, the keyring to the local login. That is good for
day‑to‑day protection and useless for recovery — a reinstalled system or a dead
disk takes the key with it. A backup therefore cannot use the same key, so it
is protected by a passphrase you supply.

```bash
tforge --backup vaults.tfbak
```

```bash
tforge --restore vaults.tfbak
```

- The passphrase is asked for twice when creating a backup and never echoed.
  It must be at least 12 characters: a backup holds every secret you have and,
  unlike the agent, it can be copied and attacked offline at leisure.
- **Losing the passphrase means losing the backup.** There is no recovery path
  and no way for anyone, including you, to open the file without it. Keep it
  somewhere other than next to the backup.
- After writing, the file is immediately read back and decrypted. A backup that
  cannot be restored is worse than none at all, because it is trusted; if the
  check fails the file is deleted and the command reports an error.
- `--backup` refuses to overwrite an existing file unless you pass `--force`.
- `--restore` **merges** by default: vaults from the backup are added, and any
  whose ID or name already exists locally are skipped and listed. A colliding
  name matters as much as a colliding ID, because the agent resolves a
  reference by either and takes the first match.
- `--restore --replace` discards the local vaults and installs the backup as-is.
  It asks for confirmation unless `-y` is given.

For automation, `TFORGE_BACKUP_PASSPHRASE` is used instead of prompting. That
is a deliberate trade-off — an environment variable is readable by other
processes of the same user — so the interactive prompt remains the default and
the variable is only consulted when it is set.

Backup file format (`TFBAK`, version 1): the passphrase is stretched with
Argon2id into a 256‑bit key, and the payload is encrypted with AES‑256‑GCM.
The KDF parameters are stored in the file, so they can be raised later without
invalidating existing backups, and they are covered by the authentication tag,
so they cannot be weakened to make an offline attack cheaper.

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

Requirements:

- **Go 1.25 or newer** (the `go` directive in `go.mod` is `1.25.0`)
- **Wails CLI v2.15.0 or newer** — older CLI builds embed a `golang.org/x/tools`
  that cannot read the export data of recent Go toolchains and fail with
  `internal error: package "..." without types was imported from "..."`:

  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0
  ```

- **.NET SDK 8 or newer**, only on Windows and only to build the Windows Hello
  helper

### Wails App (GUI)

```bash
wails dev      # live reloading UI + Go
wails build    # build production bundle
```

`wails dev` also serves the frontend at `http://localhost:34115`, where the
bound Go methods are callable from a normal browser — useful for inspecting the
UI with devtools.

### Agent & CLI

Build the agent into the repository root so it finds the Windows Hello helper
in `helper-bin/` without extra configuration:

```bash
go build -o tforge-agent.exe ./cmd/tforge-agent
```

```bash
go build -o tforge.exe ./cmd/tforge
```

With `go run` the binary lands in a temporary directory instead, so the helper
has to be pointed at explicitly via `TFORGE_HELLO_HELPER` (see the agent
section above).

### Windows Hello helper

```powershell
./build-hello-helper.ps1
```

Publishes `hello-helper/TForge.HelloHelper` as a self-contained single-file
executable and copies it to `helper-bin/tforge-hello-helper.exe`. Pass
`-OutDir` to place it elsewhere — the installer uses this to put the helper
next to the installed agent.

### Tests

```bash
go test ./...
```

Covers the env-file parser and export quoting, the agent's lock behaviour and
local-client guard, the vault service's copy semantics, the AES-GCM round trip
including tampering and wrong-key cases, and the refusal to persist after a
failed load.

### Install scripts

`install-tforge-tools.ps1` (Windows/PowerShell) builds `tforge`,
`tforge-agent` and the Windows Hello helper into `~/.tforge/bin`, adds that
directory to your PATH and sets up auto-start for the agent.
`install-tforge-tools.sh` does the equivalent on Linux and macOS.

---

## Installation on Windows (CLI + Agent)

Requirements:

- Go 1.25+ installed and on `PATH`
- PowerShell (default on Windows 10+)
- .NET SDK 8+ on `PATH`, for the Windows Hello helper

Steps:

1. Open a PowerShell window in the project root (`tforge`).
2. Run the install script:

   ```powershell
   ./install-tforge-tools.ps1
   ```

   - Default install path is `~\.tforge\bin`.
   - This directory is added to your user `PATH` so `tforge` and `tforge-agent`
     are available in new terminals.
   - The script also builds `tforge-hello-helper.exe` and places it next to the
     agent. **Without it the agent cannot be unlocked** — it starts locked, and
     `/unlock` has no way to run the Windows Hello prompt. If the .NET SDK is
     missing the script warns and continues; run `./build-hello-helper.ps1`
     afterwards to catch up.

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

- Go 1.25 or newer,
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

The ordering is driven by risk rather than by feature area. For a tool whose
only job is holding secrets, *“I can always get my data back”* comes before
everything else.

**v1 – Data integrity & platform support**

- [x] ~~OS‑backed `Protector` on Windows (DPAPI)~~
- [x] ~~OS‑backed `Protector` on macOS/Linux (Keychain / Secret Service)~~
- [x] ~~agent starts locked, with inactivity timeout and Windows Hello re‑auth on unlock~~
- [x] ~~a versioned header for the storage format, recording the format version
      and which protector sealed the payload~~
- [x] ~~encrypted, passphrase‑protected backup and restore (`--backup` /
      `--restore`), so a lost user profile or a dead disk no longer takes the
      vaults with it~~
- [ ] backup and restore from the GUI as well; today they are CLI‑only
- [ ] **a guard against concurrent writes.** The GUI and the CLI both
      read‑modify‑write the entire file without a lock. Deleting a vault from
      the CLI while the GUI is open brings it back on the GUI’s next save, and
      interleaved writes can drop entries.
- [ ] re‑auth on macOS (LocalAuthentication / Touch ID), then Linux — both are
      still stubs that always succeed, so the lock offers no protection there
- [ ] respect `Entry.Type` when building the environment; `note` entries are
      currently injected into the child process like any other key
- [ ] first‑class Linux support (packaging, autostart, desktop integration)

**v2 – Developer experience**

- [ ] more granular export modes (filter by group or type), extending
      `--export-format`
- [ ] local activity log for vault usage, without logging secret values
- [ ] **decide** the headless/CI story, then implement it. The open question is
      not packaging but trust: a CI runner has no keyring, no DPAPI and no
      Hello. Either it uses `SoftwareProtector` with a key supplied as a CI
      secret — which makes TForge a wrapper around a secret the runner already
      holds — or it generates a `.env` from an externally provided key, or
      TForge is explicitly not meant for CI. Picking one is the actual work.
- [ ] file‑based output for tools that cannot read environment variables.
      Most integrations people ask for (`docker compose`, `kubectl`,
      `terraform`) already work through `tforge exec` today; the remaining gap
      is only where a tool insists on a file.

**Later**

- [ ] vault sync across multiple machines — worth revisiting only if encrypted
      export/import turns out to be insufficient in practice. Sync means key
      exchange, conflict resolution and a trust model: a large amount of
      security‑critical surface for a benefit that export largely already
      delivers.

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

