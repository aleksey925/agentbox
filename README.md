# Agentbox

CLI for running AI agents (Claude Code, GitHub Copilot) inside an isolated Docker container.

## Why?

- **Security** — agent runs in a sandbox and cannot access files outside the project, modify system configs, or cause unintended side effects
- **Convenience** — no need to approve every agent action since it works in an isolated environment
- **Reproducibility** — consistent development environment across different machines

## Requirements

- Docker
- [mise](https://mise.jdx.dev) — project must have `mise.toml` for tools provisioning inside the container

## Installation

```bash
git clone https://github.com/aleksey925/agentbox.git
cd agentbox
bash ./agentbox.sh install
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

This copies Dockerfile and docker-compose.yml to your project and creates `mise.toml` if it doesn't exist.

### Run sandbox

```bash
agentbox run
```

Starts the container with your project mounted. Inside the container, you can run Claude Code, Copilot, or other AI agents.

### Rebuild

```bash
agentbox re-full      # Full rebuild (no cache)
agentbox re-tools     # Rebuild mise + languages + agents
agentbox re-agents    # Rebuild only AI agents (Claude, Copilot)
```

Rebuild layers:
```
re-full   → [apt packages] → [mise + languages] → [AI agents]
re-tools  →                  [mise + languages] → [AI agents]
re-agents →                                       [AI agents]
```

### Clean up

```bash
agentbox clean
```

Removes sandbox files from the project.
