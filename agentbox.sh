#!/usr/bin/env bash
#
# agentbox - CLI for Docker-based AI agent sandbox
#

set -euo pipefail

VERSION="1.0.0"
SCRIPT_NAME="$(basename "$0")"

# Installation paths
INSTALL_BIN="$HOME/.local/bin/agentbox"
INSTALL_CONF="$HOME/.agentbox"

COMPOSE_FILE="docker-compose.agentbox.yml"
DOCKERFILE="Dockerfile.agentbox"
SERVICE_NAME="agentbox"

# Colors
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    BOLD='\033[1m'
    NC='\033[0m'
else
    RED='' GREEN='' YELLOW='' BLUE='' BOLD='' NC=''
fi

log_info()    { echo -e "${BLUE}ℹ${NC} $*"; }
log_success() { echo -e "${GREEN}✓${NC} $*"; }
log_warning() { echo -e "${YELLOW}⚠${NC} $*"; }
log_error()   { echo -e "${RED}✗${NC} $*" >&2; }
die()         { log_error "$@"; exit 1; }

# escape special regex characters in a string
regex_escape() { printf '%s' "$1" | sed 's/[.[\*^$()+?{|]/\\&/g'; }

# check if running from repository (not installed)
is_repo() {
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    [[ -d "$script_dir/conf" && -d "$script_dir/.git" ]]
}

# cross-platform sed -i
sed_inplace() {
    if [[ "$OSTYPE" == darwin* ]]; then
        sed -i '' "$@"
    else
        sed -i "$@"
    fi
}

# ============================================================================
# Commands
# ============================================================================

cmd_init() {
    if [[ ! -d "$INSTALL_CONF" ]]; then
        die "Source directory not found: $INSTALL_CONF"
    fi
    
    # Check if files already exist
    local existing=()
    for f in "$COMPOSE_FILE" "$DOCKERFILE"; do
        [[ -f "$f" ]] && existing+=("$f")
    done
    
    if [[ ${#existing[@]} -gt 0 ]]; then
        log_warning "Files already exist: ${existing[*]}"
        read -rp "Overwrite? [y/N] " answer
        [[ ! "$answer" =~ ^[Yy]$ ]] && die "Aborted"
    fi
    
    log_info "Initializing agentbox..."
    
    local git_exclude=""
    [[ -f ".git/info/exclude" ]] && git_exclude=".git/info/exclude"
    
    local copied=0
    for file in "$INSTALL_CONF"/*; do
        [[ ! -f "$file" ]] && continue
        
        local filename
        filename="$(basename "$file")"
        
        if cp "$file" "./$filename"; then
            log_success "Copied: $filename"
            copied=$((copied + 1))
            
            local pattern
            pattern="$(regex_escape "$filename")"
            if [[ -n "$git_exclude" ]] && ! grep -q "^${pattern}$" "$git_exclude" 2>/dev/null; then
                echo "$filename" >> "$git_exclude"
                log_info "Added to .git/info/exclude: $filename"
            fi
        fi
    done
    
    [[ $copied -eq 0 ]] && die "No files copied"

    if [[ ! -f "mise.toml" ]]; then
        touch mise.toml
        log_success "Created: mise.toml"
    fi

    echo
    log_success "Agentbox initialized (${copied} files)"
    echo
    echo "Next steps:"
    echo -e "  ${BOLD}$SCRIPT_NAME run${NC}        - Start sandbox"
    echo -e "  ${BOLD}$SCRIPT_NAME re-full${NC}    - Full rebuild and start"
    echo -e "  ${BOLD}$SCRIPT_NAME re-tools${NC}   - Rebuild tools and start"
    echo -e "  ${BOLD}$SCRIPT_NAME re-agents${NC}  - Rebuild AI agents and start"
}

cmd_run() {
    if [[ ! -f "$COMPOSE_FILE" ]]; then
        log_warning "Not initialized, running init first..."
        cmd_init
        echo
    fi
    
    log_info "Starting agentbox..."
    exec docker compose -f "$COMPOSE_FILE" run --rm "$SERVICE_NAME"
}

cmd_rebuild() {
    local mode="${1:-}"
    local run_after="${2:-false}"
    
    if [[ ! -f "$COMPOSE_FILE" ]]; then
        log_warning "Not initialized, running init first..."
        cmd_init
        echo
    fi
    
    local build_args=()
    local cache_bust
    cache_bust="$(date +%s)"
    
    case "$mode" in
        full)
            log_info "Full rebuild (no cache)..."
            build_args+=(--no-cache)
            ;;
        tools)
            log_info "Rebuilding tools (mise + languages) and agents..."
            build_args+=(--build-arg "REBUILD_TOOLS=$cache_bust")
            ;;
        agents)
            log_info "Rebuilding AI agents (Claude, Copilot)..."
            build_args+=(--build-arg "REBUILD_AGENTS=$cache_bust")
            ;;
        *)
            die "Unknown rebuild mode: $mode"
            ;;
    esac
    
    docker compose -f "$COMPOSE_FILE" build "${build_args[@]}" "$SERVICE_NAME"
    log_success "Build complete"
    
    if [[ "$run_after" == "true" ]]; then
        echo
        log_info "Starting agentbox..."
        exec docker compose -f "$COMPOSE_FILE" run --rm "$SERVICE_NAME"
    fi
}

cmd_clean() {
    log_info "Cleaning agentbox files..."

    local git_exclude=".git/info/exclude"
    local removed=0
    for f in "$COMPOSE_FILE" "$DOCKERFILE"; do
        if [[ -f "$f" ]]; then
            rm -f "$f" && log_success "Removed: $f"
            removed=$((removed + 1))
        fi

        local pattern
        pattern="$(regex_escape "$f")"
        if [[ -f "$git_exclude" ]] && grep -q "^${pattern}$" "$git_exclude" 2>/dev/null; then
            sed_inplace "/^${pattern}$/d" "$git_exclude"
            log_info "Removed from $git_exclude: $f"
        fi
    done

    [[ $removed -eq 0 ]] && log_warning "No files to remove" || log_success "Cleaned ${removed} files"
}

cmd_install() {
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    local conf_dir="$script_dir/conf"

    if [[ ! -d "$conf_dir" ]]; then
        die "Config directory not found: $conf_dir"
    fi

    local existing=()
    [[ -f "$INSTALL_BIN" ]] && existing+=("$INSTALL_BIN")
    [[ -d "$INSTALL_CONF" ]] && existing+=("$INSTALL_CONF/")

    if [[ ${#existing[@]} -gt 0 ]]; then
        log_warning "Already installed: ${existing[*]}"
        read -rp "Reinstall? [y/N] " answer
        [[ ! "$answer" =~ ^[Yy]$ ]] && die "Aborted"
    fi

    log_info "Installing agentbox..."

    mkdir -p "$(dirname "$INSTALL_BIN")"
    cp "$script_dir/agentbox.sh" "$INSTALL_BIN"
    chmod +x "$INSTALL_BIN"
    log_success "Installed: $INSTALL_BIN"

    mkdir -p "$INSTALL_CONF"
    local copied=0
    for file in "$conf_dir"/*; do
        [[ ! -f "$file" ]] && continue
        cp "$file" "$INSTALL_CONF/"
        log_success "Copied: $(basename "$file") -> $INSTALL_CONF/"
        copied=$((copied + 1))
    done

    echo
    log_success "Installation complete (${copied} config files)"
    echo
    echo "Make sure ~/.local/bin is in your PATH:"
    echo -e "  ${BOLD}export PATH=\"\$HOME/.local/bin:\$PATH\"${NC}"
    echo
    echo "Add shell completions:"
    echo -e "  ${BOLD}eval \"\$(agentbox completions bash)\"${NC}  # for bash"
    echo -e "  ${BOLD}eval \"\$(agentbox completions zsh)\"${NC}   # for zsh"
}

cmd_completions() {
    local shell="${1:-bash}"
    local cmd_name="${2:-agentbox}"
    [[ -z "$cmd_name" ]] && cmd_name="agentbox"
    
    case "$shell" in
        bash)
            cat <<'EOF'
_agentbox() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local commands="init run re-full re-tools re-agents clean help completions"
    COMPREPLY=($(compgen -W "$commands" -- "$cur"))
}
complete -F _agentbox agentbox
EOF
            if [[ -n "$cmd_name" && "$cmd_name" != "agentbox" ]]; then
                echo "complete -F _agentbox $cmd_name"
            fi
            ;;
        zsh)
            cat <<'EOF'
_agentbox() {
    local commands="init run re-full re-tools re-agents clean help completions"
    _arguments "1:command:($commands)"
}
compdef _agentbox agentbox
EOF
            if [[ -n "$cmd_name" && "$cmd_name" != "agentbox" ]]; then
                echo "compdef _agentbox $cmd_name"
            fi
            ;;
        *)
            die "Unknown shell: $shell (supported: bash, zsh)"
            ;;
    esac
}

cmd_help() {
    local install_cmd=""
    is_repo && install_cmd="
    install      Install agentbox to ~/.local/bin"

    echo -e "${BOLD}agentbox${NC} v${VERSION} - Docker sandbox for AI agents

${BOLD}USAGE${NC}
    $SCRIPT_NAME <command>

${BOLD}COMMANDS${NC}
    init         Initialize sandbox in current directory
    run          Start sandbox container

  ${BOLD}Rebuild:${NC}
    re-full      Full rebuild (no cache) and start
    re-tools     Rebuild tools (mise + languages) and start
    re-agents    Rebuild AI agents (Claude, Copilot) and start

  ${BOLD}Other:${NC}
    clean        Remove sandbox files from project${install_cmd}
    completions  Output shell completions
    help         Show this help

${BOLD}REBUILD LAYERS${NC}
    re-full   → [apt packages] → [mise + languages] → [AI agents]
    re-tools  →                  [mise + languages] → [AI agents]
    re-agents →                                       [AI agents]

${BOLD}EXAMPLES${NC}
    cd ~/my-project
    $SCRIPT_NAME init          # Initialize
    $SCRIPT_NAME run           # Start container
    $SCRIPT_NAME re-full       # Full rebuild
    $SCRIPT_NAME re-tools      # Update tools (mise) + agents
    $SCRIPT_NAME re-agents     # Update Claude/Copilot


${BOLD}COMPLETIONS${NC}
    # Bash: add to ~/.bashrc
    eval \"\$($SCRIPT_NAME completions bash)\"

    # Zsh: add to ~/.zshrc
    eval \"\$($SCRIPT_NAME completions zsh)\"

    # With alias:
    alias abox=agentbox
    eval \"\$($SCRIPT_NAME completions zsh abox)\"
"
}

case "${1:-help}" in
    init)        cmd_init ;;
    run|start)   cmd_run ;;
    re-full)     cmd_rebuild full true ;;
    re-tools)    cmd_rebuild tools true ;;
    re-agents)   cmd_rebuild agents true ;;
    clean|rm)    cmd_clean ;;
    install)     cmd_install ;;
    completions) cmd_completions "${2:-}" "${3:-}" ;;
    help|-h|--help) cmd_help ;;
    -v|--version)   echo "agentbox v${VERSION}" ;;
    *)           die "Unknown command: $1. Run '$SCRIPT_NAME help'" ;;
esac
