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

The global skeleton at `~/.agentbox/skeleton/` is the source of all static sandbox
configuration. A project's `.agentbox/` directory is a plain copy of it - never a merge
or an overlay. Files sit flat with no nested directories and carry a version in their
name (e.g. `core.v1.yml`) so a new version shows up as a new file the user can diff
against. Updates are explicit: nothing auto-updates, the user reinitializes when they
choose.

Why: a single, copy-only source makes the same skeleton always produce the same project
config, and the user fully owns and can freely edit the template.

Deviation: a project's `local.yml` holds project-specific overrides and is never
overwritten on reinit.

Deviation: configuration that depends on per-launch state is generated live rather than
copied from the skeleton - see "The agent cannot rewrite its own sandbox" and "Live, not
baked". The skeleton stays the source for everything static.

### The agent cannot rewrite its own sandbox

The project is mounted read-write so the agent can edit its code, but the files that define
the sandbox itself are layered read-only on top of that mount. The project's `.agentbox/`
(Dockerfile, compose) is never agent-writable.

Why: an agent that could rewrite its own jail - by accident or after being hijacked - would
weaken the next build and break containment. The read-only layer holds even against
in-sandbox root, since remounting it needs a capability the container does not carry. The
user still owns these files and edits them from the host.

The same protection extends to a git project's exec surface - `.git/hooks` and `.git/config`
(which can point at hooks via `core.hooksPath`, filters, or `fsmonitor`). Writing either
would run code on the host the next time git touches the repo, so both are mounted
read-only. config staying read-only also confines the agent to the remotes the user already
configured. This overlay is generated live per launch rather than copied from the skeleton,
because it is conditional: it applies only when the project is a git repo, and the real git
dir is resolved at launch (it can sit outside the project in worktrees and submodules, where
it is not mounted and so needs no protection).

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
  The CLI exports this path as `AGENTBOX_PROJECT_PATH`; the compose file requires it
  (`:?`) rather than falling back to `${PWD}`. A direct `docker compose` run leaves it
  unset and fails loudly, because such a run also skips the launcher's protections (the
  read-only git exec-surface overlay, project-readiness checks) and would otherwise mount
  a weaker sandbox at a guessed path without warning.

### Presets terminology

The same concept is named differently by audience: "sandbox configuration" for the whole,
"environment presets" for the Go/Python components that mount host caches, "development
tools" in user-facing UI, and `Preset` in code.

Why: each audience needs the term that reads clearest to it. Mixing the terms either
confuses users or muddies the code.

### Download integrity

A downloaded agent binary becomes executable inside the sandbox with every secret and
directory mounted, so it is verified against a vendor-published SHA-256 before install
whenever one exists for the exact asset we fetch. The archive is streamed to a temp file
while hashed and is only extracted after the hash matches; a mismatch removes the temp file
and fails the install. The checksum is fetched over the same channel as the binary, so this
guards against a corrupted or partially-tampered release, not a full takeover of the host
that serves both - the same trust model the Claude download already uses. An agent with no
published checksum streams straight to extraction instead of buffering: buffering an archive
we cannot verify would add latency without adding any safety.

Coverage is dictated by what each vendor actually publishes for the asset we download, not
by choice:

- `claude` verifies a SHA-256 from its release manifest.
- `copilot` and `ralphex` verify against the `SHA256SUMS`-style file in their GitHub
  release. A missing entry is a hard error, never a silent skip, so a truncated or
  wrong-version checksums file cannot downgrade to an unverified install.
- `codex`, `opencode`, `cursor` are unverified: codex publishes only a sigstore bundle for
  this asset (no plain checksum), opencode checksums only its desktop builds, and cursor
  publishes nothing and has its version scraped from a live install script. Pinning hashes
  in-repo is not an option because the version is resolved live ("latest") on each install.

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
