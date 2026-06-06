# Agentbox Development Guide

## CLI Architecture

This project uses a **data-driven CLI** pattern inspired by urfave/cli. The `commandTree()` function in `commands_meta.go` is the **single source of truth** for all commands, subcommands, and flags. Everything else (router, help, completions, tests) derives from it automatically.

### Command Structure

```
agentbox <command> [subcommand] [flags] [arguments]
```

### Adding a New Command

1. **Add entry to `commandTree()` in `commands_meta.go`:**

```go
func commandTree() []Command {
    return []Command{
        // ... existing commands ...
        {
            Name:        "mycommand",
            Description: "Short description for help",
            Handler:     (*App).cmdMyCommand,
            Flags: []Flag{
                {"-s, --short", "Short flag description"},
                {"--long-only", "Long-only flag description"},
            },
        },
    }
}
```

2. **Implement handler in `commands.go`:**

```go
func (a *App) cmdMyCommand(args []string) int {
    if hasHelpFlag(args) {
        fmt.Printf(`%s

Usage:
  agentbox mycommand [flags] <required-arg>

Arguments:
  required-arg                      Description of required argument

Flags:
  -s, --short                       Short flag description
  --long-only                       Long-only flag description
`, CommandDesc("mycommand"))
        return 0
    }

    // Get allowed flags from commandTree (defined once!)
    flags := CommandFlags()["mycommand"]
    if code := RejectUnknownFlagsWithAllowed(args, flags); code != 0 {
        return code
    }

    // Command logic using lazy-cached resources
    paths, err := a.Paths()
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        return 1
    }

    // ... command logic ...
    return 0
}
```

**That's it!** Router, completions, and help are automatically generated from `commandTree()`.

### Adding Subcommands

```go
{
    Name:        "parent",
    Description: "Parent command description",
    Handler:     (*App).cmdParentStatus,  // default action, or nil if subcommand required
    Subcommands: []Command{
        {Name: "sub1", Description: "First subcommand", Handler: (*App).cmdParentSub1},
        {Name: "sub2", Description: "Second subcommand", Handler: (*App).cmdParentSub2, Flags: []Flag{
            {"--flag", "Subcommand-specific flag"},
        }},
    },
},
```

If `Handler` is `nil`, the command requires a subcommand. Help is auto-generated for parent-only commands.

### Lazy-Cached Resources

Use `App` methods instead of creating resources directly:

```go
// Instead of:
paths, err := config.NewPaths()
manager, err := agents.NewManager(paths)

// Use:
paths, err := a.Paths()        // cached
manager, err := a.AgentManager()  // cached
```

Resources are created once and reused across subcommands.

## Flag Conventions

| Convention            | Example         | Description                           |
|-----------------------|-----------------|---------------------------------------|
| Short first           | `-a, --all`     | Short form always before long form    |
| Single dash for short | `-a`            | Single character flags                |
| Double dash for long  | `--all`         | Multi-character flags                 |
| No value flags        | `--verbose`     | Boolean flags don't take values       |
| Value flags           | `--output file` | Flags with values use space separator |

### Flag Validation

```go
// Get flags from commandTree - single source of truth
flags := CommandFlags()["mycommand"]
if code := RejectUnknownFlagsWithAllowed(args, flags); code != 0 {
    return code
}
```

## Help Format

### Main Help (`agentbox --help`)

Auto-generated from `commandTree()`. Shows all commands with descriptions.

### Command Help (`agentbox <command> --help`)

```
Command short description

Usage:
  agentbox command [flags] [arguments]

Arguments:
  arg-name                          Description (column at position 36)

Flags:
  -s, --short                       Description (column at position 36)

Examples:
  agentbox command arg              Description of example
```

Rules:
- Description column starts at position 36
- Short flag first: `-s, --short`
- Do NOT include `-h, --help` in Flags section
- Use `<required>` and `[optional]` brackets

## Error Messages

```go
fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
fmt.Fprintf(os.Stderr, "Error: %v\n", err)
return 1
```

## Exit Codes

| Code | Meaning                                   |
|------|-------------------------------------------|
| 0    | Success                                   |
| 1    | Error (invalid args, runtime error, etc.) |

## Naming Conventions

- **Commands**: singular nouns (`agent`, `completion`) or verbs (`init`, `run`, `attach`)
- **Subcommands**: verbs (`update`, `use`)
- **Flags**: lowercase, hyphen-separated (`--no-cache`, `--build-no-cache`)
- **Handlers**: `cmd` + command path (`cmdInit`, `cmdAgentUpdate`, `cmdSelfUninstall`)

## Environment Presets Terminology

Sandbox supports multiple development tools (Go, Python, etc.) via **environment presets**.
Presets mount host caches and configs into the sandbox for better performance.

### Terminology by Context

| Context                | Term                  | Example                                    |
|------------------------|-----------------------|--------------------------------------------|
| **Global concept**     | Sandbox configuration | "Update sandbox configuration"             |
| **Components**         | Environment presets   | "Available presets: Go, Python"            |
| **UI (user-friendly)** | Development tools     | "Select your development tools"            |
| **Code (internal)**    | `Preset`              | `type Preset struct`, `SupportedPresets()` |

### Examples

**UI Prompt:**
```
Configure sandbox
Select your development tools — sandbox will mount
their caches and configs for better performance
```

**Help text:**
```
Update sandbox configuration (change enabled Go, Python presets)
```

## Skeleton Architecture

Skeleton is the user's global sandbox configuration stored in `~/.agentbox/skeleton/`. It contains Docker Compose files and Dockerfile that define the sandbox environment.

### Directory Structure

```
~/.agentbox/skeleton/           # global skeleton (user-owned)
├── core.v1.yml                 # base compose config (always present)
├── go.v1.yml                   # Go preset (optional)
├── python.v1.yml               # Python preset (optional)
├── Dockerfile.v1.agentbox      # sandbox Dockerfile
└── local.yml                   # user customizations template

project/.agentbox/              # project sandbox (copied from skeleton)
├── core.v1.yml
├── go.v1.yml
├── Dockerfile.v1.agentbox
└── local.yml                   # project-specific customizations (never overwritten)
```

### Core Concepts

| Concept             | Description                                                                 |
|---------------------|-----------------------------------------------------------------------------|
| **Skeleton**        | Global template at `~/.agentbox/skeleton/`, user fully owns and can edit    |
| **Presets**         | Environment configs (Go, Python) that mount host caches into sandbox        |
| **local.yml**       | Project-specific overrides, preserved during reinit                         |
| **Versioned files** | `*.v1.yml`, `Dockerfile.v1.agentbox` — version in name for tracking changes |

### Design Principles

1. **Single source of truth** — skeleton is the only source, no merging or layering
2. **Flat structure** — all files at one level, no nested directories
3. **User ownership** — user can freely edit skeleton files
4. **Explicit updates** — no auto-updates, user controls when to reinit with `--force`
5. **Deterministic** — same skeleton always produces same project config
6. **Safe local.yml** — project's `local.yml` is never overwritten

### Commands

| Command                          | Behavior                                                                                    |
|----------------------------------|---------------------------------------------------------------------------------------------|
| `agentbox init`                  | If no skeleton → TUI for preset selection → create skeleton → copy to project               |
| `agentbox init`                  | If skeleton exists → clean project's `.agentbox/` (except `local.yml`) → copy from skeleton |
| `agentbox init skeleton`         | Error if skeleton exists (suggests `--force`)                                               |
| `agentbox init skeleton --force` | TUI with current presets pre-selected → recreate skeleton after confirmation                |

### File Versioning

Files use version suffix (`v1`) to help users track changes:
- `core.v1.yml` — when we release `core.v2.yml`, user sees a new file appeared
- User can compare versions and migrate customizations
- Old versions can be manually removed after migration

## Agent Launch Flags

Flags that agents (harnesses) are launched with are **user configuration, not
build-time constants**. They live in a global, user-owned file and are resolved
**live on every launch** by the in-sandbox launcher — so changing them never
requires an image rebuild, and edits apply to the next launch even inside a
running sandbox.

Design principles:

1. **No imposed defaults** — out of the box no extra flags are passed to any
   agent. Cautious users are never surprised by a permissive mode they didn't
   pick. `SuggestedFlags` is the hook for shipping recommended defaults later.
2. **Global, not per-project** — one setting for all projects, like agent
   binaries themselves.
3. **Live, not baked** — the launcher reads the flags file on each invocation;
   flags are deliberately kept out of the image to avoid rebuilds.
4. **Bash-readable format** — a simple line-based format (one agent per line,
   `*` for all) rather than YAML, because the launcher is plain bash with no
   YAML parser available.

## Code Style

- All comments in English
- Use `fmt.Fprintf(os.Stderr, ...)` for errors
- Use `fmt.Printf(...)` or `fmt.Println(...)` for normal output
- Group imports: stdlib, then external, then internal
