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
name (e.g. `core.v2.yml`) so a new version shows up as a new file the user can diff
against. Updates are explicit: nothing auto-updates, the user reinitializes when they
choose.

Why: a single, copy-only source makes the same skeleton always produce the same project
config, and the user fully owns and can freely edit the template.

Deviation: a project's `local.yml` holds project-specific overrides and is never
overwritten on reinit.

Deviation: configuration that depends on per-launch state is generated live rather than
copied from the skeleton - see "The agent cannot rewrite its own sandbox" and "Live, not
baked". The skeleton stays the source for everything static.

### Config schema version and upgrade

Each managed file carries a version in its name (`core.vN.yml`, `go.vN.yml`,
`Dockerfile.vN.agentbox`). The bump rule is delivery, not pure compatibility:
**bump a file's version whenever its change must reach the user** - a breaking
change to the sandbox contract, or a backward-compatible one we still want
adopted (a security hardening). A purely cosmetic edit keeps the same name and
no version. The binary embeds exactly one set of versions, so a project's
`.agentbox/` and the binary are one coupled unit: the binary cannot run an
older compose, because runtime state it injects (the live project path, the
git-protection overlay) has no meaning in the old layout.

Detection is per-file: `run` compares every managed file the project has
against the embedded version. Mandatory files (core, Dockerfile) and any preset
present are checked; a preset the project lacks is ignored, so a preset bump
never blocks a project that does not use it. When anything is older, `run`
refuses to start and points at `agentbox upgrade` - a hard block, not a
warning, so a mismatched sandbox is never mounted silently. The gate compares
against the binary, not the skeleton, so a freshly regenerated skeleton never
masks a stale project.

`upgrade` is the single delivery channel - it reseeds from the current
templates, so it carries both breaking and non-breaking changes; only the
gate's force differs. Without a path it upgrades the current project and
rebuilds its image. With a path it scans that directory (`--depth`, default 1)
for projects, reseeds each, and drops the shared `agentbox:local` image so
every project rebuilds on its next run. There is no registry and no
auto-update: non-breaking changes reach a project only when the user runs
`upgrade`; breaking ones arrive the same way but the gate makes running it
unavoidable. `init` applies the same gate to the global skeleton - it refuses
to seed a project from a stale skeleton and points at `init skeleton --force`,
rather than mutating global state under the hood.

Two deliberate consequences. Presets are an install-wide choice, not
per-project (the skeleton holds one set), so `upgrade` aligns every project to
the current global set - it can add or drop a preset from a project. And the
shared `agentbox:local` tag is safe only because the image bakes no project
content; keep image builds project-independent or the shared tag breaks.

Why: the version turns an unavoidable forced migration into an explicit,
auditable one. Staying on an old schema means pinning the old binary (the two
move together); deviating from the current schema means `local.yml` overrides,
which survive upgrades - not freezing a version. For a security tool, keeping
the weaker old jail runnable would be a misfeature, so the forced step is
intended; the version only makes it safe and visible. The one irreducible
discipline is judging whether a change must be delivered - when unsure, bump.

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
configured. The surface includes git dirs nested inside `.git`: each submodule git dir
under `.git/modules/` carries its own hooks and config, and each worktree's
`config.worktree` under `.git/worktrees/` can set `core.hooksPath` - a hook planted there
runs on the host just like a top-level one. A missing surface entry is created empty at
launch so it can be mounted; skipped, the agent could create it from inside the sandbox.
This overlay is generated live per launch rather than copied from the skeleton, because it
is conditional: it applies only when the project is a git repo, and the real git dir is
resolved at launch (it can sit outside the project in worktrees and submodules, where it is
not mounted and so needs no protection). The check fails closed: a `.git` that exists but
cannot be resolved (git missing, `safe.directory` refusal) aborts the launch instead of
silently starting an unprotected sandbox.

### Container capabilities

The container is the security boundary, so the agent is free inside it: it runs as a
non-root user with passwordless sudo, which keeps in-container work convenient (e.g.
`apt install`). sudo is not a host risk, because root inside is still bounded by the
container's Linux capability set - sudo can reach that ceiling, never exceed it.

So the lever is the capability set, not sudo. Docker's default set already omits the
dangerous ones (`SYS_ADMIN`, `NET_ADMIN`, `SYS_MODULE`, ...); on top of that agentbox drops
`NET_RAW` and `MKNOD`, which a normal dev workflow never needs and only sudo-as-root could
otherwise use. The cost is small and explicit - no raw sockets, so `ping`/`tcpdump` stop
working; a project that needs them adds the capability back in its own `local.yml`.

The rest is kept on purpose: `SETUID`/`SETGID`/`CHOWN`/`DAC_OVERRIDE`/`FOWNER`/`FSETID`/
`SETFCAP` because apt and sudo need them, `SYS_CHROOT` and `NET_BIND_SERVICE` because
chroot builds and binding privileged ports are occasional but real dev needs. `AUDIT_WRITE`
is kept too: dropping it only stops the container forging entries in the host audit log
(negligible on a dev machine) while making sudo print an `unable to send audit message`
warning on every call. `no-new-privileges` is intentionally not set: it blocks setuid and
so would break sudo - trimming capabilities is the chosen lever instead.

A generous `pids_limit` is set alongside as a cheap guard against a fork bomb exhausting
the host's process table - high enough not to bite real parallel builds, and overridable in
`local.yml`. Memory and CPU caps are deliberately left unset by default, because a fixed
limit would OOM-kill legitimate heavy builds; a project sets them in its own `local.yml`.

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
  YAML, because the launcher is plain bash with no YAML parser. The per-agent wrapper
  scripts that read this file are installed in root-owned `/usr/local/bin`, not the user's
  `~/.local/bin`: Claude Code's self-update points `~/.local/bin/claude` at its own binary
  and would clobber a wrapper there, silently dropping the flags. `/usr/local/bin` precedes
  `~/.local/bin` in PATH and the sandbox user cannot write it without sudo, so the wrapper
  wins and survives a self-update; `DISABLE_AUTOUPDATER=1` stops the update churn on top.
- The project is mounted at the same absolute path it has on the host, not a fixed
  location. Agents key per-project state by working directory (e.g. Claude Code stores
  `--resume` history under a path derived from the cwd), so mirroring the host path gives
  each project its own history and lines it up with non-sandbox runs of the same agent.
  The CLI exports this path as `AGENTBOX_PROJECT_PATH`; the compose file requires it
  (`:?`) rather than falling back to `${PWD}`. A direct `docker compose` run leaves it
  unset and fails loudly, because such a run also skips the launcher's protections (the
  read-only git exec-surface overlay, project-readiness checks) and would otherwise mount
  a weaker sandbox at a guessed path without warning.

### Masking project sub-directories

The whole project is mounted read-write, but some sub-directories must be
hidden instead: a host `.venv` or `node_modules` built with macOS binaries is
incompatible inside the Linux container and breaks the build if mounted.
Masking replaces such a directory with its own isolated, initially-empty Docker
volume - the host copy is never touched and the agent works in its own. This is
also containment: the agent cannot corrupt the host's copy or un-mask itself.

The mechanism is the nested-mount trick the `.agentbox:ro` and git overlays
already use - a volume mounted at a deeper path wins over the project bind
regardless of compose order. The volume is per-project and persistent (not
tmpfs, not anonymous), so it stays warm across `run --rm`; its name
`agentbox-mask-<projhash>-<subhash>` stops two projects sharing one, and the
`<projhash>` prefix is what orphan cleanup filters on. Mounted on a path the
image lacks it is created `root:root`, so a generic `.bashrc` loop chowns each
masked path to `box:box` once while still root-owned. The paths arrive via
`AGENTBOX_MASK_PATHS` (newline-joined, since a path may contain spaces or
colons), the same live channel as `AGENTBOX_PROJECT_PATH`; the loop is a no-op
when empty.

The list lives in `.agentbox/masked-dirs` (one path per line, `#` comments),
read-only like the rest of `.agentbox/` so the agent cannot un-mask itself. It
is generated, not a skeleton template, because its content depends on
per-project detection: on a fresh seed init/upgrade auto-detect candidates
(`.venv`, `venv`, `.tox`, `node_modules`) at the root and one level deep, write
the active ones and the rest as commented examples, and preserve an existing
file on reinit. The fragment is appended only in `Run`, never `Build`, so the
shared image stays project-independent, and cleanup self-heals: each run drops
volumes whose lines the user removed, best-effort.

Masking fits only host-built artifacts the container rebuilds anyway. A
directory the tooling rebuilds in place by deleting and recreating it - Go's
`vendor/` via `go mod vendor` - is deliberately excluded: masking turns it into
a mount point that cannot be removed (`go mod vendor` then fails with EBUSY),
and buys no compatibility since `vendor/` is portable source. It stays
unmasked; a user who wants it masked adds the line by hand.

Limitation on Docker Desktop (macOS): the mask volume nests inside the project
bind, which goes through a live file-sharing layer. Deleting a masked directory
on the host *while the sandbox runs* detaches the volume - the path reverts to
the project bind and containment is lost until the next launch. It is not
agent-exploitable (inside the sandbox the masked path is the volume, so
deleting it removes volume content, not the host directory); only a host-side
`rm` during a live session triggers it. The only robust fix - binding each
top-level entry separately - would stop new top-level files from syncing live,
a worse tradeoff. The contract: while a sandbox runs, never delete a masked
directory node (inside the sandbox it fails with EBUSY anyway); to rebuild
contents, clear them in place rather than deleting the node.

### Presets terminology

The same concept is named differently by audience: "sandbox configuration" for the whole,
"environment presets" for the Go/Python language components, "development tools" in
user-facing UI, and `Preset` in code.

Why: each audience needs the term that reads clearest to it. Mixing the terms either
confuses users or muddies the code.

### Preset caches are sandbox-local

A preset's dependency cache (Go module cache, uv cache) is a named Docker volume shared
across agentbox projects, never a bind mount of the host's real cache - the same scheme the
mise and opencode caches already use.

Why: the host cache is executable surface. Go does not re-verify already-extracted module
sources on each build, so a writable bind mount lets an agent poison a cached module that
the host's `go` later compiles. A sandbox-local volume keeps the cache writable (the agent
can still add dependencies) and warm across runs, while a poisoned entry can never reach the
host. The cost is a one-time re-download per volume; a project that deliberately wants the
host cache mounts it in its own `local.yml`.

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
- `copilot`, `ralphex`, `pi`, and `codex` verify against the `SHA256SUMS`-style file in
  their GitHub release. A missing entry is a hard error, never a silent skip, so a
  truncated or wrong-version checksums file cannot downgrade to an unverified install.
  `pi` and `codex` download a whole archive, so the checksum covers the entire tree
  rather than one binary - `pi` because its binary loads siblings by relative path like
  `cursor`, `codex` because the package bundle is its only checksummed asset (agentbox
  then trims that bundle, see "Agent install layout").
- `opencode` and `cursor` are unverified: opencode checksums only its desktop builds, and
  cursor publishes nothing and has its version scraped from a live install script. Pinning
  hashes in-repo is not an option because the version is resolved live ("latest") on each
  install.
- `self update` replaces the running binary, so it verifies against the release's
  goreleaser `checksums.txt` the same way, and validates the target version before it
  reaches a URL.

Both the buffered download and the decompressed output are bounded by a fixed cap, so a
gzip bomb or an endless stream can exhaust neither disk nor the host - the extractor fails
closed once the limit is crossed.

Extraction is confined the same way. Every write goes through an `os.Root` opened on the
destination, so a path that resolves outside it is refused by the kernel-level check rather
than by a name comparison. Checking the entry name alone is not enough: the name is cleaned
lexically, and an archive can decouple it from where the entry lands - a symlink entry
pointing at the destination itself makes a later entry nested "under" it look contained
while the write follows the link outside. Symlink entries are still rejected up front when
their target is absolute or climbs out of the archive root, because `os.Root` guards
traversal, not the link text an archive asks to store.

That up-front check is lexical, so it cannot be the last word on a symlink: `x/up -> ..`
followed by `leak -> x/up/../evil` cleans to a path inside the destination while really
pointing at its parent, and the entry that decides it can come after the link. So once the
entry loop ends, every symlink the archive created is probed again through the `os.Root` -
the target text uncleaned, so the root resolves `..` against the real parent the way the
kernel will - and one that lands outside is removed and fails the extraction. Only a target
that resolves nowhere is left alone: it cannot escape, and archives legitimately name a
target they never ship. A link that escapes is worth failing on even when nothing is written
through it, because it stays in the installed tree for anything that later follows it.

The second pass runs however the loop ends, a failing entry included - the entry that fails
can be the one right after the escaping link, and returning early would leave that link on
disk. The first error is the one returned, so the caller reads why extraction failed rather
than a verification error standing in for it. Containment is therefore the extractor's own
guarantee, not something a caller earns by deleting the destination on error.

### Agent install layout

The in-sandbox launcher execs one fixed path per agent, `<agent>/<version>/<binary>`, so
that path has to be a real binary and the version directory a complete install behind it.
`cursor` and `pi` keep their vendor layout verbatim, because they load siblings by relative
path. `codex` deliberately does not: its package bundle keeps the binary one level down in
`bin/`, so agentbox promotes the contents of `bin/` to the version root and drops the rest
of the bundle. That puts `codex-code-mode-host` where codex looks for it - next to its own
executable - without teaching the launcher a second path shape.

The dropped parts cost nothing: the image's apt `ripgrep` is the `rg` codex used before,
`bwrap` cannot create a namespace without `CAP_SYS_ADMIN` (see "Container capabilities"),
and the bundled `zsh` is unused. The bundle is still the download source only because it
is the one codex asset with a published checksum.

Only regular files are promoted - anything else in `bin/` fails the install. Promotion
moves an entry one directory up, and that rebases every relative symlink under it: a
target the extractor cleared as inside the version directory (see "Download integrity")
resolves one level higher once promoted, and two such links chained together climb
arbitrarily far. Checking the shape is enough because the version directory is defined as
the bundle's binaries and nothing else. The cost is that a bundle which ever ships a
symlink or a subdirectory in `bin/` stops installing until agentbox learns the new shape -
the right direction to fail for a path that ends in an exec.

An existing version directory normally skips the download: for most agents the directory
existing is the whole check, and an agent that needs more opts in by implementing the
layout check itself. codex does - it rejects a directory written by an older layout - and
the caller then drops that directory and re-seeds it.

Why: a bare binary that fails closed on a missing sibling is a broken install that still
looks complete. A completed version directory is reused as-is rather than re-downloaded
into; a rejected one is cleared before the re-seed, never extracted over, or the install
would end up a mix of two layouts. The re-seed is non-atomic, so without the
reject-and-reseed step a layout fix would reach a user only once the vendor happened to
publish a new version. A stale install is reported with the command that repairs it
wherever agentbox meets one - `agent use`, `agent`, `init`, and every `run`,
including the ones that attach to a container already up - rather than repaired behind the
user's back, because an unannounced ~110 MB download inside `agentbox run` costs a session
while an ignored warning costs a line of stderr. The same warning covers any agent whose
selected version directory is simply gone, and any agent whose directory holds anything at
all with no version selected - either way the launcher has nothing to exec, and reinstalling
is the same remedy. Only a directory that is absent or empty stays silent, deliberately: a
never-installed agent and an install that failed before it wrote anything are the same
state, so warning would fire for every agent the user chose not to install. That is also
exactly the test that decides whether a first run installs everything, so a directory
counted as installed can never be silently skipped here. The warning says the remedy
plainly - `agent update` installs the latest version, so a user sitting on a deliberately
pinned older one knows the advice costs them the pin.

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
