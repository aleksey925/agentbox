Agentbox
========

CLI for running AI agents (Claude Code, GitHub Copilot, OpenAI Codex, Gemini CLI) inside an isolated Docker container.

- [Why use Agentbox?](#why-use-agentbox)
- [Installation](#installation)
  - [Shell Completions](#shell-completions)
- [Updating](#updating)
- [How to Use](#how-to-use)
  - [Language Support](#language-support)
  - [Customization](#customization)

## Why use Agentbox?

- **Security** — agents run in a sandbox and cannot access files outside the project, modify system configs, or cause unintended side effects
- **Convenience** — no need to approve every agent action since it works in an isolated environment

## Installation

Download the latest release from [releases](https://github.com/aleksey925/agentbox/releases) and install it manually
or you can run the following commands to install the latest version to `~/.local/bin`:

```bash
VERSION=$(curl -sL -o /dev/null -w '%{url_effective}' https://github.com/aleksey925/agentbox/releases/latest | sed 's/.*\/v//')
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -#L "https://github.com/aleksey925/agentbox/releases/download/v${VERSION}/agentbox_${VERSION}_${OS}_${ARCH}.tar.gz" | tar xz -C ~/.local/bin
```

Also, you can build it from source:

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

## Updating

Agentbox can update itself. Run `agentbox self update <version>` to update to a specific version,
or use `agentbox self update <tab>` to choose a version and install it.

## How to Use

Navigate to your project directory and run `agentbox init`. On first run, you'll be prompted to
select which programming languages to enable:

```
$ agentbox init

No skeleton found. Let's set it up...

Detecting environment...

Enable languages:
[x] Go      (detected: $GOPATH is set)
[x] Python  (detected: ~/.cache/uv exists)

Enter numbers to toggle (e.g., 1,3 or 1-3), or press Enter to accept:
```

The following files will be created in your project's `.agentbox/` directory:

- `core.v*.yml` — main compose configuration
- `<lang>.v*.yml` — language-specific configurations (e.g., `go.v1.yml`, `python.v1.yml`)
- `Dockerfile.agentbox` — container image definition
- `local.yml` — your personal overrides (never overwritten)
- `mise.toml` — configuration for [mise](https://mise.jdx.dev) tool manager (in project root)

The `.agentbox/` directory is automatically added to `.git/info/exclude` to keep it out of version control.

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

### Language Support

Agentbox uses modular templates for language support. Each language includes all common tool caches:

| Language | Environment Variables                                                       | Volumes                            |
|----------|-----------------------------------------------------------------------------|------------------------------------|
| Go       | `GOPATH=/home/box/go`                                                       | `$GOPATH:/home/box/go`             |
| Python   | `UV_PROJECT_ENVIRONMENT=/home/box/.venv`<br>`VENV_DIR_PATH=/home/box/.venv` | `~/.cache/uv`, `~/.cache/pypoetry` |

To change language selection after initial setup, run `agentbox init skeleton`. This will backup
your current skeleton to `~/.agentbox/skeleton.backup/` and let you re-select languages.

### Customization

There are two levels of customization:

**Simple customization** — edit `.agentbox/local.yml` in your project:

```yaml
services:
  agentbox:
    volumes:
      - ./data:/home/box/data          # mount additional directory
      - ~/.ssh:/home/box/.ssh:ro       # mount SSH keys (read-only)
    environment:
      - MY_API_KEY=secret
```

The `local.yml` file is never overwritten by agentbox — your changes are safe.

**Advanced customization** — edit files in `~/.agentbox/skeleton/`:

The skeleton directory contains the templates used to generate project configurations.
You can modify any file there, and changes will be applied when you run `agentbox init`
in new projects. This is useful for organization-wide customizations.
