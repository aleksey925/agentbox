Agentbox
========

CLI for running AI agents (Claude Code, GitHub Copilot, OpenAI Codex, Gemini CLI) inside an isolated Docker container.

- [Why use Agentbox?](#why-use-agentbox)
- [Installation](#installation)
  - [Shell Completions](#shell-completions)
- [How to Use](#how-to-use)

## Why use Agentbox?

- **Security** — agents run in a sandbox and cannot access files outside the project, modify system configs, or cause unintended side effects
- **Convenience** — no need to approve every agent action since it works in an isolated environment

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

Agentbox supports shell completions for Bash and Zsh. To enable them, add one of the following
lines to your shell configuration:

```bash
# Bash: add to ~/.bashrc
eval "$(agentbox completion bash)"

# Zsh: add to ~/.zshrc
eval "$(agentbox completion zsh)"
```

If you use an alias for agentbox, pass the alias name as the second argument:

```bash
# For alias "abox"
alias abox="agentbox"
eval "$(agentbox completion bash abox)"
```

## How to Use

Navigate to your project directory and run `agentbox init`. This command creates several files in your 
project and downloads AI agent binaries to `~/.agentbox/bin/`.

The following files will be added to your project:

- `Dockerfile.agentbox` — defines the container image. This file is overwritten on every `agentbox init`, so do not modify it manually.
- `docker-compose.agentbox.yml` — main compose configuration with volume mounts and environment variables. This file is also overwritten on every `agentbox init`.
- `docker-compose.agentbox.local.yml` — your personal overrides. This file is created only once and never overwritten. Use it to add custom volumes, environment variables, or any other Docker Compose settings you need.
- `mise.toml` — configuration for [mise](https://mise.jdx.dev) tool manager. Created only if it doesn't exist. Use it to specify which tools (Python, Node.js, Go, etc.) should be available inside the container.

All these files are automatically added to `.git/info/exclude` to keep them out of version control.

After initialization, run `agentbox run` to start the container. Your project is mounted at `/home/box/app` inside 
the container. AI agents are available as commands with permissive flags enabled:

```bash
claude    # runs with --dangerously-skip-permissions
copilot   # runs with --allow-all-paths --allow-all-tools
codex     # runs with --full-auto
gemini    # runs with --yolo
```

To rebuild the container image before running, use `agentbox run --build`. For a full rebuild
without Docker cache, use `agentbox run --build-no-cache`.

To list running containers, use `agentbox ps`. To attach to an already running container, use
`agentbox attach` (interactive selection) or `agentbox attach <container-id>`.

Agent binaries are managed separately from the container. Use `agentbox agent` to see installed versions
vs latest available. Use `agentbox agent update` to update all agents, or `agentbox agent update claude copilot`
to update specific ones. To switch to a specific version, use `agentbox agent use claude 2.0.67`.

To remove all agentbox files from the project, run `agentbox clean`.
