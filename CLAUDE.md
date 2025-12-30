# Agentbox Development Guide

## CLI Design Conventions

This project follows modern CLI design patterns inspired by Docker, kubectl, and git.

### Command Structure

```
agentbox <command> [subcommand] [flags] [arguments]
```

### Adding a New Command

1. **Add command handler in `cli.go`:**

```go
case "mycommand":
    return app.cmdMyCommand(cmdArgs)
```

2. **Implement command in `commands.go`:**

```go
func (a *App) cmdMyCommand(args []string) int {
    // 1. Check for help flag first
    if hasHelpFlag(args) {
        fmt.Print(`Short description of command

Usage:
  agentbox mycommand [flags] <required-arg> [optional-arg]

Arguments:
  required-arg                      Description of required argument
  optional-arg                      Description of optional argument

Flags:
  -s, --short                       Short flag with long form
  --long-only                       Long-only flag
`)
        return 0
    }

    // 2. Parse and validate flags
    for _, arg := range args {
        switch arg {
        case "-s", "--short":
            // handle flag
        default:
            if strings.HasPrefix(arg, "-") {
                fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
                fmt.Fprintf(os.Stderr, "Available flags: -s, --short\n")
                return 1
            }
        }
    }

    // 3. Command logic
    return 0
}
```

3. **Update main help in `cli.go` `printHelp()`** — only add command name and short description, no flags

4. **Add shell completions in `completions.go`**

5. **Write tests in `commands_test.go`**

### Flag Conventions

| Convention | Example | Description |
|------------|---------|-------------|
| Short first | `-a, --all` | Short form always before long form |
| Single dash for short | `-a` | Single character flags |
| Double dash for long | `--all` | Multi-character flags |
| No value flags | `--verbose` | Boolean flags don't take values |
| Value flags | `--output file` | Flags with values use space separator |

### Flag Validation

**IMPORTANT:** All commands and subcommands MUST validate flags using the centralized helpers:

```go
// For commands with NO flags (only positional args):
func (a *App) cmdMyCommand(args []string) int {
    if hasHelpFlag(args) {
        // ... show help ...
        return 0
    }

    if code := RejectUnknownFlags(args); code != 0 {
        return code
    }

    // ... command logic ...
}

// For commands WITH allowed flags:
var myCommandAllowedFlags = []string{"-a", "--all", "--verbose"}

func (a *App) cmdMyCommand(args []string) int {
    if hasHelpFlag(args) {
        // ... show help ...
        return 0
    }

    if code := RejectUnknownFlagsWithAllowed(args, myCommandAllowedFlags); code != 0 {
        return code
    }

    // ... parse known flags and command logic ...
}
```

**Centralized validation functions** (in `commands_meta.go`):
- `RejectUnknownFlags(args)` — for commands with no flags
- `RejectUnknownFlagsWithAllowed(args, allowedFlags)` — for commands with flags
- `ValidateNoUnknownFlags(args, allowedFlags)` — low-level validation, returns error

Rules:
- ALWAYS use centralized validation helpers
- NEVER manually check for unknown flags with ad-hoc code
- Define allowed flags as package-level variables for consistency
- Help flags (`-h`, `--help`) are always allowed automatically

### Main Help Format (`agentbox --help`)

```
agentbox VERSION - CLI tool for running AI agents in Docker sandbox

Usage:
  agentbox <command> [options]

Commands:
  command1                          Short description
  command2                          Short description

Global Flags:
  -h, --help                        Show help
  -v, --version                     Show version

Use "agentbox <command> --help" for more information about a command.
```

**Main help rules:**
- Only command names and short descriptions
- No command-specific flags — those go in `<command> --help`
- Global flags section at the end

### Command Help Format (`agentbox <command> --help`)

```
Command short description

Usage:
  agentbox command [flags] [arguments]

Arguments:
  arg-name                          Description (column at position 36)

Flags:
  -s, --short                       Description (column at position 36)

Additional notes if needed.

Examples:
  agentbox command arg              Description of example
```

**Command help rules:**
- Description column starts at position 36
- Short flag first: `-s, --short`
- Do NOT include `-h, --help` in Flags section (it's a global flag)
- Omit Flags section entirely if command has no flags
- Use `<required>` and `[optional]` brackets
- Use `...` for variadic arguments: `[arg...]`
- Subcommands should also support `--help` (e.g., `agentbox agent update --help`)

### Error Messages

```go
// Unknown flag
fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", arg)
fmt.Fprintf(os.Stderr, "Available flags: -a, --all\n")
return 1

// Unknown subcommand
fmt.Fprintf(os.Stderr, "Unknown %s subcommand: %s\n", command, subcmd)
return 1

// Missing required argument
fmt.Fprintf(os.Stderr, "Usage: agentbox command <required>\n")
return 1

// Runtime error
fmt.Fprintf(os.Stderr, "Error: %v\n", err)
return 1
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Error (invalid args, runtime error, etc.) |

### Naming Conventions

- **Commands**: singular nouns (`agent`, `completion`) or verbs (`init`, `run`, `attach`)
- **Subcommands**: verbs (`update`, `use`)
- **Flags**: lowercase, hyphen-separated (`--no-cache`, `--build-no-cache`)

### Testing Commands

**Important:** Use centralized metadata in `commands_meta.go` for consistency:
- `AllCommands()` — list of all top-level commands
- `CommandFlags()` — valid flags for each command
- `AgentSubcommands()` — agent subcommands
- `CompletionShells()` — valid shells

When adding a new command:
1. Add to `AllCommands()` in `commands_meta.go`
2. Add flags to `CommandFlags()` if any
3. Synchronization tests will automatically verify completions match

```go
func TestCmdMyCommand(t *testing.T) {
    // arrange
    app := &App{Version: "test"}

    // act
    code := app.cmdMyCommand([]string{"--help"})

    // assert
    if code != 0 {
        t.Errorf("expected exit code 0, got %d", code)
    }
}
```

### Synchronization Tests

Tests in `commands_test.go` automatically verify:

**Bash completion:**
- `TestBashCompletionContainsAllCommands` — has all commands from `AllCommands()`
- `TestBashCompletionContainsAllFlags` — has all flags from `CommandFlags()`
- `TestBashCompletionContainsAllAgentSubcommands` — has all subcommands from `AgentSubcommands()`
- `TestBashCompletionContainsAllAgentNames` — has all agents from `agents.AllAgentNames()`
- `TestBashCompletionContainsAllShells` — has all shells from `CompletionShells()`

**Zsh completion:**
- `TestZshCompletionContainsAllCommands` — has all commands from `AllCommands()`
- `TestZshCompletionContainsAllFlags` — has all flags from `CommandFlags()`
- `TestZshCompletionContainsAllAgentSubcommands` — has all subcommands from `AgentSubcommands()`
- `TestZshCompletionContainsAllAgentNames` — has all agents from `agents.AllAgentNames()`
- `TestZshCompletionContainsAllShells` — has all shells from `CompletionShells()`

**CLI behavior:**
- `TestCliRouterHandlesAllCommands` — router handles all commands from `AllCommands()`
- `TestAllCommandsRejectUnknownFlags` — all top-level commands reject unknown flags
- `TestAllSubcommandsRejectUnknownFlags` — all subcommands reject unknown flags (auto-generated from `AllSubcommandPaths()`)
- `TestAgentSubcommandHelp` — subcommands show their own help, not parent help
- `TestHelpExitCode` — help returns exit code 0

If you add a command to `AllCommands()` but forget to update completions, tests will fail.

### Adding Subcommands

When adding a new subcommand:
1. Add it to `AgentSubcommands()` (or equivalent for other parent commands)
2. Add a test path to `AllSubcommandPaths()` in `commands_meta.go`

Test cases for flag validation are **auto-generated** from `AllSubcommandPaths()` — no need to manually add test cases.

## Code Style

- All comments in English
- Use `fmt.Fprintf(os.Stderr, ...)` for errors
- Use `fmt.Printf(...)` or `fmt.Println(...)` for normal output
- Group imports: stdlib, then external, then internal
