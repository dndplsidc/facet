#!/bin/bash
#
# Creates mock package manager binaries in $HOME/mock-bin.
# These log install commands instead of actually installing.
# PATH is set by the harness — only the child process sees these mocks.
set -euo pipefail

mkdir -p "$HOME/mock-bin"
MOCK_PKG_LOG="$HOME/.mock-packages"
touch "$MOCK_PKG_LOG"

# Mock package managers unless Docker E2E is intentionally exercising real ones.
if [ "${FACET_E2E_REAL_PACKAGES:-}" != "1" ]; then
    # ── Mock brew ──
    cat > "$HOME/mock-bin/brew" << 'BREWEOF'
#!/bin/bash
MOCK_PKG_LOG="$HOME/.mock-packages"
case "$1" in
    install)
        shift
        for arg in "$@"; do
            [[ "$arg" == --* ]] && continue
            echo "$arg" >> "$MOCK_PKG_LOG"
            echo "mock-brew: installed $arg"
        done
        ;;
    list)
        cat "$MOCK_PKG_LOG" 2>/dev/null
        ;;
    --prefix)
        echo "/opt/homebrew"
        ;;
    *)
        echo "mock-brew: $*"
        ;;
esac
exit 0
BREWEOF
    chmod +x "$HOME/mock-bin/brew"

    # ── Mock apt-get ──
    cat > "$HOME/mock-bin/apt-get" << 'APTEOF'
#!/bin/bash
MOCK_PKG_LOG="$HOME/.mock-packages"
case "$1" in
    install)
        shift
        for arg in "$@"; do
            [[ "$arg" == -* ]] && continue
            echo "$arg" >> "$MOCK_PKG_LOG"
            echo "mock-apt: installed $arg"
        done
        ;;
    update)
        echo "mock-apt: updated"
        ;;
    *)
        echo "mock-apt: $*"
        ;;
esac
exit 0
APTEOF
    chmod +x "$HOME/mock-bin/apt-get"

    # ── Mock sudo (passes through to the command) ──
    cat > "$HOME/mock-bin/sudo" << 'SUDOEOF'
#!/bin/bash
"$@"
SUDOEOF
    chmod +x "$HOME/mock-bin/sudo"
else
    echo "[mock-tools] Using real package manager"
fi

# ── Mock npx (for AI skills) ──
MOCK_AI_LOG="$HOME/.mock-ai"
touch "$MOCK_AI_LOG"

cat > "$HOME/mock-bin/npx" << 'NPXEOF'
#!/bin/bash
MOCK_AI_LOG="$HOME/.mock-ai"
if [ "$1" = "--version" ]; then
    echo "10.0.0"
    exit 0
fi
if [ "$1" = "skills" ]; then
    echo "npx $*" >> "$MOCK_AI_LOG"
    echo "mock-npx: skills $*"
    exit 0
fi
echo "mock-npx: $*"
exit 0
NPXEOF
chmod +x "$HOME/mock-bin/npx"

# ── Mock pi (for Pi extension management) ──
MOCK_PI_LOG="$HOME/.mock-pi"
touch "$MOCK_PI_LOG"

cat > "$HOME/mock-bin/pi" << 'PIEOF'
#!/bin/bash
MOCK_PI_LOG="$HOME/.mock-pi"
MOCK_PI_EXTENSIONS="$HOME/.mock-pi-extensions"
touch "$MOCK_PI_EXTENSIONS"

echo "pi $*" >> "$MOCK_PI_LOG"

if [ "$1" = "install" ]; then
    name="$2"
    if [ -n "${NPM_CONFIG_REGISTRY:-}" ]; then
        echo "pi install env NPM_CONFIG_REGISTRY=$NPM_CONFIG_REGISTRY" >> "$MOCK_PI_LOG"
    fi
    grep -qx "$name" "$MOCK_PI_EXTENSIONS" 2>/dev/null || echo "$name" >> "$MOCK_PI_EXTENSIONS"
    echo "mock-pi: install $name"
    exit 0
fi

if [ "$1" = "remove" ]; then
    name="$2"
    grep -vx "$name" "$MOCK_PI_EXTENSIONS" > "$MOCK_PI_EXTENSIONS.tmp" || true
    mv "$MOCK_PI_EXTENSIONS.tmp" "$MOCK_PI_EXTENSIONS"
    echo "mock-pi: remove $name"
    exit 0
fi

if [ "$1" = "list" ]; then
    cat "$MOCK_PI_EXTENSIONS"
    exit 0
fi

echo "mock-pi: $*"
exit 0
PIEOF
chmod +x "$HOME/mock-bin/pi"

# ── Mock claude (for Claude Code MCP registration) ──
# Tracks registered MCP names in $HOME/.mock-claude-mcps so that duplicate
# `mcp add` calls fail with "already exists", matching real CLI behavior.
cat > "$HOME/mock-bin/claude" << 'CLAUDEEOF'
#!/bin/bash
MOCK_AI_LOG="$HOME/.mock-ai"
MOCK_MCP_STATE="$HOME/.mock-claude-mcps"
touch "$MOCK_MCP_STATE"
echo "claude $*" >> "$MOCK_AI_LOG"

if [ "$1" = "mcp" ] && [ "$2" = "add" ]; then
    name="$3"
    if grep -qx "$name" "$MOCK_MCP_STATE" 2>/dev/null; then
        echo "MCP server $name already exists in local config" >&2
        exit 1
    fi
    echo "$name" >> "$MOCK_MCP_STATE"
    echo "mock-claude: mcp add $name"
    exit 0
fi

if [ "$1" = "mcp" ] && [ "$2" = "remove" ]; then
    name="$3"
    if [ -f "$MOCK_MCP_STATE" ]; then
        grep -vx "$name" "$MOCK_MCP_STATE" > "$MOCK_MCP_STATE.tmp" || true
        mv "$MOCK_MCP_STATE.tmp" "$MOCK_MCP_STATE"
    fi
    echo "mock-claude: mcp remove $name"
    exit 0
fi

echo "mock-claude: $*"
exit 0
CLAUDEEOF
chmod +x "$HOME/mock-bin/claude"

echo "[mock-tools] AI tools mocked in $HOME/mock-bin"
