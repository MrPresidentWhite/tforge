#!/usr/bin/env bash
set -euo pipefail

# Simple installer for non-Windows systems.
# - can be run inside a cloned repo OR remotely via curl|bash
# - builds tforge and tforge-agent
# - installs them into a user bin directory (default: ~/.local/bin)
# - optionally sets up a systemd user service for tforge-agent

REPO_URL="${REPO_URL:-https://github.com/MrPresidentWhite/tforge.git}"

INSTALL_DIR="${1:-"$HOME/.local/bin"}"

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

echo
echo "Done."
echo "You can now use tforge, for example:"
echo "  tforge --env dev @MyVault npm run dev"

