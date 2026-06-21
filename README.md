# AgentBox

<p align="left">
  <img src="img/logo.png" alt="Agentbox" width="500">
</p>

CLI for running AI agents (Claude Code, GitHub Copilot, OpenAI Codex, Cursor CLI, OpenCode, Ralphex, Pi) inside an isolated Docker container.

- [Why use Agentbox?](#why-use-agentbox)
- [Installation](#installation)
  - [Shell Completions](#shell-completions)
- [Updating](#updating)
- [How to Use](#how-to-use)
  - [Modular Sandbox Configuration](#modular-sandbox-configuration)
  - [Customization](#customization)
- [Development](#development)

## Why use Agentbox?

- **Security** — agents run in an isolated Docker container scoped to your
  project; the rest of your machine - other files, your host system and its
  settings - stays out of reach
- **Peace of mind** — let the agent work freely without reviewing every step,
  since it can't change your host system or files outside the project
- **One CLI for every agent** — run Claude Code, Copilot, Codex, Cursor, OpenCode,
  Ralphex or Pi through the same commands, and pin or update each agent's version
  without touching your host
- **Consistent, fast environment** — a reproducible toolchain on every run, with
  presets that reuse your host's package caches so dependencies aren't re-downloaded

## Installation

The easiest way is via [Homebrew](https://brew.sh):

```bash
brew install aleksey925/apps/agentbox
```

Alternatively, download the latest release from [releases](https://github.com/aleksey925/agentbox/releases) and install
it manually, or run the following commands to install the latest version to `~/.local/bin`:

```bash
VERSION=$(curl -sL -o /dev/null -w '%{url_effective}' https://github.com/aleksey925/agentbox/releases/latest | sed 's/.*\/v//')
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
curl -#L "https://github.com/aleksey925/agentbox/releases/download/v${VERSION}/agentbox_${VERSION}_${OS}_${ARCH}.tar.gz" | tar xz -C ~/.local/bin agentbox
```

Also, you can [build from source](#build).

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

If you installed via Homebrew, update with `brew upgrade agentbox`.

Otherwise agentbox can update itself. Run `agentbox self update <version>` to update to a specific version,
or use `agentbox self update <tab>` to choose a version and install it.

After updating the binary, bring your project sandboxes to the new version with `agentbox upgrade`
(see [Updating Configuration](#updating-configuration)). A project on an outdated config refuses to
`run` until you do, so nothing starts on a mismatched sandbox.

## How to Use

```bash
cd your-project
agentbox init    # set up sandbox (configure presets on first run) and download agents for first time
agentbox run     # start sandbox
```

By default agents run with no extra flags. To always launch an agent with specific
flags — for example to bump verbosity, pin a model, or enable a permissive mode —
configure them globally:

```bash
agentbox agent flags          # open the flags file in $EDITOR
agentbox agent flags --show   # print current flags
agentbox agent flags --path   # print the flags file path
```

The flags file (`~/.agentbox/flags/agent-flags`) is global and read live on every
launch: edits apply to the next agent run — even inside a running sandbox — with no
image rebuild. One line per agent, `*` applies to any agent without its own line:

```
claude --verbose
cursor --no-color
* --some-shared-flag
```

Each agent's own flags are documented in its CLI (`<agent> --help`); agentbox just
forwards whatever you put here.

Your project is mounted inside the sandbox at the same absolute path it has on the host.
This keeps each project's agent session history (e.g. `claude --resume`) separate per project,
and shared with non-sandbox runs of the same agent.

**Git Configuration**

Your `~/.gitconfig` is automatically mounted into the sandbox (read-only), so git commits work
with your identity. If you haven't configured git globally yet, run:

```bash
git config --global user.name "Your Name"
git config --global user.email "your@email.com"
```

**Other Commands**

| Command                         | Description                                          |
| ------------------------------- | ---------------------------------------------------- |
| `agentbox run`                  | Start sandbox, or attach if one is already running   |
| `agentbox run --new`            | Force a new container even if one is running         |
| `agentbox run --container <id>` | Attach to a specific container by name or ID         |
| `agentbox run --build`          | Rebuild image and start a new container              |
| `agentbox run --build-no-cache` | Full rebuild without cache                           |
| `agentbox ps`                   | List running sandboxes                               |
| `agentbox upgrade`              | Migrate the current project to this version's config |
| `agentbox upgrade <path>`       | Migrate every project found under a path             |
| `agentbox clean`                | Remove sandbox files from project                    |

**Managing Agents**

Agent binaries are managed separately from the sandbox:

```bash
agentbox agent                      # show installed vs latest versions
agentbox agent update               # update all agents
agentbox agent update claude        # update specific agent
agentbox agent use claude 2.0.67    # switch to specific version
agentbox agent flags                # edit flags agents are launched with
```

### Modular Sandbox Configuration

Sandbox configuration is modular — it consists of a core config (`core.v4.yml`) plus environment
presets (like `go.v2.yml`) you select during `agentbox init`. Presets give the sandbox a warm,
isolated tool cache, so dependencies aren't re-downloaded on every run.

Available presets: `Go`, `Python`.

### Customization

Agentbox stores your sandbox configuration in `~/.agentbox/skeleton/`:

```
~/.agentbox/skeleton/           # your global skeleton (you own this)
├── core.v4.yml                 # base sandbox config
├── go.v2.yml                   # Go preset (if selected)
├── python.v3.yml               # Python preset (if selected)
├── Dockerfile.v4.agentbox      # sandbox Dockerfile
└── local.yml                   # template for project customizations
```

You can freely edit any files in skeleton — they will be copied to projects on `agentbox init`.

#### Project Configuration

Each project gets a `.agentbox/` directory copied from your skeleton:

```
project/.agentbox/
├── core.v4.yml
├── go.v2.yml
├── Dockerfile.v4.agentbox
├── masked-dirs                 # project sub-dirs hidden from the sandbox (never overwritten)
└── local.yml                   # project-specific overrides (never overwritten)
```

- **`local.yml`** — add project-specific settings here, this file is never overwritten
- **`masked-dirs`** — list project sub-directories to hide from the sandbox;
  each is replaced inside the container by its own isolated, empty volume.
  Detected `.venv` and `node_modules` are masked by default, so host-built
  artifacts (macOS binaries) never reach the Linux container and the container
  builds its own copy. Never overwritten once created.
  - **Masking does not fit every directory.** It suits host-built artifacts the
    container must rebuild anyway (a macOS `.venv`, a platform `node_modules`).
    It does not suit a directory your tooling rebuilds in place by deleting and
    recreating it - Go's `vendor/`, for example. Masking turns the directory
    into a mount point, and a mount point cannot be removed: inside the sandbox
    `go mod vendor` fails with "device or resource busy", and on the host it
    cannot recreate `vendor/` while a sandbox holds it as a mount anchor. So
    `vendor/` is not masked by default; mask only host-built artifacts the
    container cannot reuse, not directories your tooling regenerates.
  - **Recreate in place, never delete the directory itself.** A masked
    directory is a mount point. While a sandbox is running, do not remove the
    directory node - inside the sandbox it fails with "device or resource
    busy", and from the host (Docker Desktop) it detaches the volume and
    re-exposes the host path until you restart. To rebuild what is inside - for
    example to recreate a `.venv` - clear its contents and rebuild in place
    instead of deleting and recreating the folder. To replace the directory
    node itself, stop the sandbox first and do it between runs.
- All `.yml` files are automatically merged when running the sandbox

#### Updating Configuration

Managed files carry a version in their name (`core.v4.yml`). A new agentbox release bumps it when a
change must reach you. A project still on the old version refuses to `run` and tells you to upgrade,
so a sandbox never starts on a config that does not match the binary.

| Task                                                         | Command                             |
| ------------------------------------------------------------ | ----------------------------------- |
| Migrate the current project to this version                  | `agentbox upgrade`                  |
| Migrate every project found under a path                     | `agentbox upgrade <path>`           |
| Scan deeper than one level for projects                      | `agentbox upgrade <path> --depth 2` |
| Reinit the current project from the skeleton                 | `agentbox init`                     |
| Change selected presets / recreate the skeleton from scratch | `agentbox init skeleton --force`    |

`upgrade` regenerates the skeleton at the current version (keeping your presets) and reseeds project
configs; `local.yml` is always preserved. Without a path it also rebuilds the current project's image;
with a path it drops the shared image so every project rebuilds on its next `run`.

## Development

### Prerequisites

- [mise](https://mise.jdx.dev/getting-started.html#installing-mise-cli) for managing toolchains

### Set up environment

- install toolchains and deps

  ```bash
  mise trust && mise install
  make deps
  ```

- verify the setup by running tests

  ```bash
  make test
  ```

### Build

Two options:

- `make build` — builds the binary into `dist/`.
- `make install` — builds and installs the binary to `~/.local/bin`.
  Ensure this directory is in your `PATH`:

  ```bash
  export PATH="$HOME/.local/bin:$PATH"
  ```
