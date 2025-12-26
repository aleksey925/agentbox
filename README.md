# Agentbox

CLI for running AI agents (Claude Code, GitHub Copilot, OpenAI Codex, Gemini CLI) inside an isolated Docker container.

## Why?

- **Security** — agents run in a sandbox and cannot access files outside the project, modify system configs, or cause unintended side effects
- **Convenience** — no need to approve every agent action since it works in an isolated environment

## Supported Agents

| Agent       | Description                 |
|-------------|-----------------------------|
| Claude Code | Anthropic's Claude Code CLI |
| Copilot     | GitHub Copilot CLI          |
| Codex       | OpenAI Codex CLI            |
| Gemini      | Google Gemini CLI           |

Agentbox automatically downloads and manages agent binaries for your platform (Linux x64/arm64).

## Requirements

- Docker
- [mise](https://mise.jdx.dev) — project must have `mise.toml` for tools provisioning inside the container

## Installation

Download the latest release from [Releases](https://github.com/aleksey925/agentbox/releases) or build from source:

```bash
git clone https://github.com/aleksey925/agentbox.git
cd agentbox
make install  # copies to ~/.local/bin
```

Make sure `~/.local/bin` is in your PATH:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

### Shell Completions

```bash
# Bash: add to ~/.bashrc
eval "$(agentbox completions bash)"

# Zsh: add to ~/.zshrc
eval "$(agentbox completions zsh)"
```

## Usage

### Initialize project

```bash
cd ~/my-project
agentbox init
```

This will:
1. Copy `Dockerfile.agentbox` and `docker-compose.agentbox.yml` to your project
2. Create empty `mise.toml` if it doesn't exist
3. Add sandbox files to `.git/info/exclude`
4. Download all AI agents (on first run)

### Run sandbox

```bash
agentbox run
```

Starts the container with your project mounted. Inside the container, agents are available with permissive flags:

```bash
claude    # runs with --dangerously-skip-permissions
copilot   # runs with --allow-all-paths --allow-all-tools
codex     # runs with --full-auto
gemini    # runs with --yolo
```

### Run with rebuild

```bash
agentbox run --build            # rebuild image before running
agentbox run --build-no-cache   # full rebuild without Docker cache
```

### Attach to running container

```bash
agentbox run <container-id>
```

### Manage agents

```bash
agentbox agents                        # show status (installed vs latest)
agentbox agents update                 # update all agents
agentbox agents update claude copilot  # update specific agents
agentbox agents use claude 2.0.67      # switch to specific version
```

### Other commands

```bash
agentbox clean     # remove sandbox files from project
agentbox upgrade   # update skeleton files from embedded
agentbox version   # show version
```

## How It Works

Agentbox stores agent binaries in `~/.agentbox/bin/` and mounts them read-only into the container. 
The container runs as a non-root user (`box`) with sudo access.

Project structure after `agentbox init`:

```
my-project/
├── Dockerfile.agentbox         # container definition
├── docker-compose.agentbox.yml # compose configuration
└── mise.toml                   # tools configuration (python, node, etc.)
```

Volumes mounted into container:
- `./` → `/home/box/app` (project files)
- `~/.agentbox/bin` → `/opt/agentbox/bin` (agent binaries, read-only)
- `~/.claude.json`, `~/.claude/` → Claude config
- `~/.copilot/` → Copilot config
- `~/.codex/` → Codex config
- `~/.gemini/` → Gemini config
- `~/.go/`, `~/.cache/uv` → caches

## License

MIT
