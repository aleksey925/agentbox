Agentbox
========

CLI for running AI agents (Claude Code, GitHub Copilot, OpenAI Codex, Gemini CLI) inside an isolated Docker container.

- [Why use Agentbox?](#why-use-agentbox)
- [Installation](#installation)
  - [Shell Completions](#shell-completions)
- [Updating](#updating)
- [How to Use](#how-to-use)
  - [Modular Sandbox Configuration](#modular-sandbox-configuration)
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

```bash
cd your-project
agentbox init    # set up sandbox (configure presets on first run) and download agents for first time
agentbox run     # start sandbox
```

Inside the sandbox, AI agents are available with permissive flags:

```bash
claude    # --dangerously-skip-permissions
copilot   # --allow-all-paths --allow-all-tools
codex     # --full-auto
gemini    # --yolo
```

Your project is mounted at `/home/box/app`.

**Other Commands**

| Command                         | Description                       |
|---------------------------------|-----------------------------------|
| `agentbox run --build`          | Rebuild and run sandbox           |
| `agentbox run --build-no-cache` | Full rebuild without cache        |
| `agentbox ps`                   | List running sandboxes            |
| `agentbox attach`               | Attach to running sandbox         |
| `agentbox clean`                | Remove sandbox files from project |

**Managing Agents**

Agent binaries are managed separately from the sandbox:

```bash
agentbox agent                      # show installed vs latest versions
agentbox agent update               # update all agents
agentbox agent update claude        # update specific agent
agentbox agent use claude 2.0.67    # switch to specific version
```

### Modular Sandbox Configuration

Sandbox configuration is modular — it consists of a core config (`core.*.yml`) plus environment presets
(like `go.v<x>.yml`) you select during `agentbox init`. Presets mount tool caches from your host into
the sandbox, so dependencies aren't re-downloaded on every run.

Available presets: `Go`, `Python`.

To change enabled presets after initial setup, run `agentbox init skeleton`.

### Customization

Agentbox uses a layered configuration system:

```
~/.agentbox/skeleton/          ← system templates (managed by agentbox)
~/.agentbox/skeleton.local/    ← your global presets (never touched by agentbox)
<project>/.agentbox/           ← project config (copied from skeleton + skeleton.local)
```

#### Global Configuration (`~/.agentbox/skeleton.local/`)

Add your own compose files to `~/.agentbox/skeleton.local/compose/` — they will be copied
to every project on `agentbox init`. This is the recommended way to add global settings, such 
as supporting development environments that Agentbox doesn't support yet.

After adding files to `skeleton.local/`, run `agentbox init` in your projects to apply them.

#### Project Configuration (`<project>/.agentbox/`)

Each project has a `.agentbox/` directory with compose files. The CLI automatically discovers
all `.yml` files and merges them when running the sandbox.

- **`local.yml`** — project-specific overrides, never overwritten by agentbox
- **`core.*.yml`** — core sandbox config (required)
- **`<preset>.*.yml`** — environment presets (optional)

You can add custom compose files directly to `.agentbox/` for project-specific needs.

#### Configuration Priority

When `agentbox init` copies files to a project:
1. Files from `skeleton/` are copied first
2. Files from `skeleton.local/` overwrite on name conflict
3. `local.yml` is created only if it doesn't exist (preserves your changes)
