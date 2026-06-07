## What this is

agentbox is a CLI that runs AI coding agents inside an isolated Docker sandbox. It
targets developers who want an agent to act without approving every step, because
the container blocks access to anything outside the project. Written in Go.

## Design decisions

### Data-driven CLI

A single declarative command tree is the source of truth for every command, subcommand,
and flag. The router, help text, shell completions, and flag validation are all derived
from it: adding a command means adding one node to the tree plus its handler, never wiring
the router, help, or completions by hand. A node with no default action is how a command
declares that it requires a subcommand, and a handler rejects unknown flags using the flag
list the tree already holds.

Why: keeps the CLI surface from drifting. With one definition, help, completions, and
validation can never disagree with the commands that actually exist.

### Resources go through the App

Shared resources - filesystem paths, the agent manager, and the like - are obtained
through accessor methods on the App, which build each one once and reuse it across
commands. Handlers never construct these directly.

Why: a single invocation can pass through several subcommands. Rebuilding the same
resource each time wastes work and lets two parts of one run disagree about state; one
cached instance keeps the run internally consistent.

### CLI surface conventions

The CLI is the whole product, so its surface is kept uniform. Flags list the short form
before the long (`-a, --all`), single dash for single-character flags, double dash for
multi-character ones, lowercase and hyphen-separated; boolean flags take no value, and a
flag that takes one receives it space-separated (`--output file`). Commands are singular
nouns or verbs, subcommands are verbs, and a handler is named `cmd` followed by its
command path.
A command exits 0 on success and 1 on any error; error text goes to stderr, normal output
to stdout. Generated help aligns descriptions at a fixed column, marks required arguments
with `<>` and optional ones with `[]`, and never lists the help flag itself.

Why: a predictable surface is the point of a CLI, and auto-generated help only stays
consistent if every command follows the same rules.

### Skeleton is the single configuration source

The global skeleton at `~/.agentbox/skeleton/` is the only source of sandbox
configuration. A project's `.agentbox/` directory is a plain copy of it - never a merge
or an overlay. Files sit flat with no nested directories and carry a version in their
name (e.g. `core.v1.yml`) so a new version shows up as a new file the user can diff
against. Updates are explicit: nothing auto-updates, the user reinitializes when they
choose.

Why: a single, copy-only source makes the same skeleton always produce the same project
config, and the user fully owns and can freely edit the template.

The project copy is mounted read-only into the sandbox, so the agent can edit the rest of
its project but never the files that define its own jail (Dockerfile, compose) - it cannot
loosen the next build. The user still owns these files and edits them from the host.

Deviation: a project's `local.yml` holds project-specific overrides and is never
overwritten on reinit.

### Live, not baked

Values that vary per user or per project are resolved live on every launch by the
in-sandbox launcher, never compiled into the image.

Why: the image is shared across all projects. Baking in a per-project value would force
an image rebuild on every change and make one project's settings leak into others.
Resolving live means an edit applies to the next launch even inside a running sandbox.

This covers two cases:

- Agent launch flags live in a global, user-owned file read on each launch. No flags are
  imposed by default, so a cautious user is never surprised by a permissive mode they did
  not pick. The format is line-based (one agent per line, `*` for any agent) rather than
  YAML, because the launcher is plain bash with no YAML parser.
- The project is mounted at the same absolute path it has on the host, not a fixed
  location. Agents key per-project state by working directory (e.g. Claude Code stores
  `--resume` history under a path derived from the cwd), so mirroring the host path gives
  each project its own history and lines it up with non-sandbox runs of the same agent.
  Direct compose runs that bypass the launcher fall back to the current directory.

### Presets terminology

The same concept is named differently by audience: "sandbox configuration" for the whole,
"environment presets" for the Go/Python components that mount host caches, "development
tools" in user-facing UI, and `Preset` in code.

Why: each audience needs the term that reads clearest to it. Mixing the terms either
confuses users or muddies the code.

## Project commands

- `make build` - build the binary into `dist/`
- `make install` - build and install the binary to `~/.local/bin`
- `make lint` - run formatters and linters via prek
- `make test` - run the test suite
- `make race` - run the suite with the race detector
- `make cover` - run tests and print a coverage report
- `make deps` - tidy and vendor Go dependencies
- `make snapshot` - build a local snapshot release with goreleaser
- `make clean` - remove `dist/` and coverage files
