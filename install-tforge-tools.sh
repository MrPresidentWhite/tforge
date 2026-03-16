#!/usr/bin/env bash
set -euo pipefail

# Simple installer for non-Windows systems (Linux and macOS).
# - can be run inside a cloned repo OR remotely via curl|bash
# - builds tforge and tforge-agent
# - installs them into a user bin directory
# - optionally sets up autostart for tforge-agent

REPO_URL="${REPO_URL:-https://github.com/MrPresidentWhite/tforge.git}"

OS="$(uname -s)"

if [ "$#" -ge 1 ]; then
  INSTALL_DIR="$1"
else
  case "$OS" in
    Darwin)
      INSTALL_DIR="$HOME/bin"
      ;;
    *)
      INSTALL_DIR="$HOME/.local/bin"
      ;;
  esac
fi

echo "Installing tforge CLI and agent to: $INSTALL_DIR"
echo

# Determine where to build from:
# - if we're inside a tforge working copy (cmd/tforge exists), build in-place
# - otherwise clone a fresh copy into a temporary directory.
WORKDIR=""
if [ -d "cmd/tforge" ] && [ -d "cmd/tforge-agent" ]; then
  WORKDIR="$(pwd)"
  echo "Detected existing tforge working copy at: $WORKDIR"
else
  echo "No local tforge working copy detected; cloning from $REPO_URL"
  TMPDIR="$(mktemp -d)"
  WORKDIR="$TMPDIR"
  git clone --depth 1 "$REPO_URL" "$WORKDIR"
fi

cd "$WORKDIR"

mkdir -p "$INSTALL_DIR"

echo "Building tforge-agent..."
go build -o "${INSTALL_DIR}/tforge-agent" ./cmd/tforge-agent

echo "Building tforge..."
go build -o "${INSTALL_DIR}/tforge" ./cmd/tforge

echo
echo "Binaries installed to: $INSTALL_DIR"

# Ensure INSTALL_DIR is on PATH for the current shell session.
case ":$PATH:" in
  *":$INSTALL_DIR:"*) echo "PATH already contains $INSTALL_DIR for this session." ;;
  *)
    echo "Temporarily adding $INSTALL_DIR to PATH for this shell session."
    export PATH="$INSTALL_DIR:$PATH"
    ;;
esac

# Persist PATH change for common shells by appending to ~/.profile if needed.
PROFILE_FILE="$HOME/.profile"
if [ -w "$PROFILE_FILE" ] || [ ! -e "$PROFILE_FILE" ]; then
  if ! grep -Fq "$INSTALL_DIR" "$PROFILE_FILE" 2>/dev/null; then
    echo "Persisting PATH update in $PROFILE_FILE"
    {
      echo
      echo "# Added by install-tforge-tools.sh"
      echo "export PATH=\"$INSTALL_DIR:\$PATH\""
    } >> "$PROFILE_FILE"
  else
    echo "$INSTALL_DIR is already mentioned in $PROFILE_FILE"
  fi
else
  echo "Note: could not update $PROFILE_FILE; please ensure $INSTALL_DIR is on your PATH."
fi

echo

case "$OS" in
  Linux)
    # Optional: set up systemd user service for tforge-agent, if systemd is available.
    if command -v systemctl >/dev/null 2>&1; then
      SYSTEMD_USER_DIR="${XDG_CONFIG_HOME:-"$HOME/.config"}/systemd/user"
      mkdir -p "$SYSTEMD_USER_DIR"

      SERVICE_PATH="${SYSTEMD_USER_DIR}/tforge-agent.service"

      echo "Creating systemd user service at: $SERVICE_PATH"

      cat > "$SERVICE_PATH" <<EOF
[Unit]
Description=TForge local vault agent

[Service]
ExecStart=${INSTALL_DIR}/tforge-agent
Restart=on-failure

[Install]
WantedBy=default.target
EOF

      echo "Reloading systemd user units and enabling tforge-agent.service (if supported)..."
      if systemctl --user daemon-reload >/dev/null 2>&1; then
        systemctl --user enable --now tforge-agent.service >/dev/null 2>&1 || \
          echo "Warning: could not enable/start tforge-agent.service. You may need to run 'systemctl --user enable --now tforge-agent.service' manually."
      else
        echo "Warning: 'systemctl --user' not available; systemd user service was written but not enabled."
      fi
    else
      echo "systemctl not found; skipping systemd user service setup."
    fi
    ;;
  Darwin)
    # Optional: set up a LaunchAgents plist for tforge-agent on macOS.
    LAUNCH_AGENTS_DIR="$HOME/Library/LaunchAgents"
    mkdir -p "$LAUNCH_AGENTS_DIR"
    PLIST_PATH="$LAUNCH_AGENTS_DIR/dev.tforge.agent.plist"

    echo "Creating LaunchAgent at: $PLIST_PATH"

    cat > "$PLIST_PATH" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key>
    <string>dev.tforge.agent</string>
    <key>ProgramArguments</key>
    <array>
      <string>${INSTALL_DIR}/tforge-agent</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${HOME}/Library/Logs/tforge-agent.log</string>
    <key>StandardErrorPath</key>
    <string>${HOME}/Library/Logs/tforge-agent.err.log</string>
  </dict>
</plist>
EOF

    echo "Loading LaunchAgent via launchctl (if possible)..."
    if command -v launchctl >/dev/null 2>&1; then
      launchctl unload "$PLIST_PATH" >/dev/null 2>&1 || true
      launchctl load -w "$PLIST_PATH" >/dev/null 2>&1 || \
        echo "Warning: could not load LaunchAgent. You may need to run 'launchctl load -w \"$PLIST_PATH\"' manually."
    else
      echo "launchctl not found; LaunchAgent plist created but not loaded."
    fi
    ;;
  *)
    echo "No automatic autostart integration for OS: $OS"
    ;;
esac

echo
echo "Done."
echo "You can now use tforge, for example:"
echo "  tforge --env dev @MyVault npm run dev"

