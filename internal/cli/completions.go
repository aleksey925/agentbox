package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/aleksey925/agentbox/internal/agents"
)

func (a *App) cmdCompletion(args []string) int {
	if len(args) == 0 || hasHelpFlag(args) {
		fmt.Print(`Generate shell completion script

Usage:
  agentbox completion <shell> [alias]

Arguments:
  shell                             Shell type: bash, zsh
  alias                             Command name for aliases (optional)

Examples:
  agentbox completion bash
  agentbox completion zsh
  agentbox completion bash abox     # for alias 'abox'

To enable completions, add to your shell config:
  # Bash (~/.bashrc)
  eval "$(agentbox completion bash)"

  # Zsh (~/.zshrc)
  eval "$(agentbox completion zsh)"
`)
		if len(args) == 0 {
			return 1
		}
		return 0
	}

	if code := RejectUnknownFlags(args); code != 0 {
		return code
	}

	shell := args[0]
	cmdName := "agentbox"
	if len(args) > 1 && args[1] != "" {
		cmdName = args[1]
	}

	switch shell {
	case "bash":
		fmt.Print(generateBashCompletion(cmdName))
	case "zsh":
		fmt.Print(generateZshCompletion(cmdName))
	default:
		fmt.Fprintf(os.Stderr, "Unknown shell: %s\n", shell)
		fmt.Fprintf(os.Stderr, "Supported shells: bash, zsh\n")
		return 1
	}

	return 0
}

func generateBashCompletion(cmdName string) string {
	agentNames := agents.AllAgentNames()
	agentNamesStr := strings.Join(agentNames, " ")
	agentNamesPattern := strings.Join(agentNames, "|")

	commands := strings.Join(AllCommands(), " ")
	runFlags := strings.Join(CommandFlags()["run"], " ")
	psFlags := strings.Join(CommandFlags()["ps"], " ")
	agentSub := strings.Join(AgentSubcommands(), " ")
	shells := strings.Join(CompletionShells(), " ")

	tmpl := `_{{.FuncName}}() {
    local cur prev commands agent_sub agent_names run_flags ps_flags
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    commands="{{.Commands}}"
    agent_sub="{{.AgentSub}}"
    agent_names="{{.AgentNames}}"
    run_flags="{{.RunFlags}}"
    ps_flags="{{.PsFlags}}"

    case "$prev" in
        {{.CmdName}})
            COMPREPLY=($(compgen -W "$commands" -- "$cur"))
            ;;
        run)
            COMPREPLY=($(compgen -W "$run_flags" -- "$cur"))
            ;;
        attach)
            local containers=$(docker ps --filter "label=com.docker.compose.service=agentbox" --filter "label=com.docker.compose.project.working_dir=$(pwd)" --format "{{.ID}}" 2>/dev/null)
            COMPREPLY=($(compgen -W "$containers" -- "$cur"))
            ;;
        ps)
            COMPREPLY=($(compgen -W "$ps_flags" -- "$cur"))
            ;;
        agent)
            COMPREPLY=($(compgen -W "$agent_sub" -- "$cur"))
            ;;
        update)
            COMPREPLY=($(compgen -W "$agent_names" -- "$cur"))
            ;;
        use)
            COMPREPLY=($(compgen -W "$agent_names" -- "$cur"))
            ;;
        {{.AgentNamesPattern}})
            if [[ "${COMP_WORDS[COMP_CWORD-2]}" == "use" ]]; then
                local versions=$(ls ~/.agentbox/bin/"$prev"/ 2>/dev/null | grep -v current)
                COMPREPLY=($(compgen -W "$versions" -- "$cur"))
            fi
            ;;
        completion)
            COMPREPLY=($(compgen -W "{{.Shells}}" -- "$cur"))
            ;;
    esac
}
complete -F _{{.FuncName}} {{.CmdName}}
`
	funcName := "_" + sanitizeFuncName(cmdName)
	result := strings.ReplaceAll(tmpl, "{{.FuncName}}", funcName)
	result = strings.ReplaceAll(result, "{{.CmdName}}", cmdName)
	result = strings.ReplaceAll(result, "{{.Commands}}", commands)
	result = strings.ReplaceAll(result, "{{.AgentSub}}", agentSub)
	result = strings.ReplaceAll(result, "{{.AgentNames}}", agentNamesStr)
	result = strings.ReplaceAll(result, "{{.AgentNamesPattern}}", agentNamesPattern)
	result = strings.ReplaceAll(result, "{{.RunFlags}}", runFlags)
	result = strings.ReplaceAll(result, "{{.PsFlags}}", psFlags)
	result = strings.ReplaceAll(result, "{{.Shells}}", shells)
	return result
}

func generateZshCompletion(cmdName string) string {
	// build agent_names array for zsh
	agentNames := agents.AllAgentNames()
	agentDescs := agents.AgentDescriptions()
	agentEntries := make([]string, 0, len(agentNames))
	for _, name := range agentNames {
		agentEntries = append(agentEntries, fmt.Sprintf("'%s:%s'", name, agentDescs[name]))
	}
	agentNamesZsh := strings.Join(agentEntries, "\n        ")

	base := `_agentbox() {
    local -a commands agent_cmds agent_names shells run_flags ps_flags

    commands=(
        'init:Initialize sandbox in current directory'
        'run:Start a new container'
        'attach:Attach to running container'
        'ps:List running agentbox containers'
        'agent:Manage AI agents'
        'clean:Remove sandbox files from project'
        'completion:Generate shell completion script'
        'help:Show help'
        'version:Show version'
    )

    run_flags=(
        '--build:Rebuild image before running'
        '--build-no-cache:Rebuild image without Docker cache'
    )

    ps_flags=(
        '--all:Show containers from all projects'
        '-a:Show containers from all projects'
    )

    agent_cmds=(
        'update:Update agents to latest version'
        'use:Switch agent to specific version'
    )

    agent_names=(
        {{.AgentNamesZsh}}
    )

    shells=(
        'bash:Bash shell'
        'zsh:Zsh shell'
    )

    local cmd=${words[2]}
    local subcmd=${words[3]}

    case $CURRENT in
        2)
            _describe -t commands 'command' commands
            ;;
        3)
            case $cmd in
                run)
                    _describe -t flags 'flag' run_flags
                    ;;
                attach)
                    local -a containers
                    containers=(${(f)"$(docker ps --filter 'label=com.docker.compose.service=agentbox' --filter "label=com.docker.compose.project.working_dir=$(pwd)" --format '{{.ID}}:{{.Names}}' 2>/dev/null)"})
                    (( ${#containers} )) && _describe -t containers 'container' containers
                    ;;
                ps)
                    _describe -t flags 'flag' ps_flags
                    ;;
                agent)
                    _describe -t commands 'agent command' agent_cmds
                    ;;
                completion)
                    _describe -t shells 'shell' shells
                    ;;
            esac
            ;;
        4)
            case $cmd in
                agent)
                    case $subcmd in
                        update)
                            _describe -t agents 'agent' agent_names
                            ;;
                        use)
                            _describe -t agents 'agent' agent_names
                            ;;
                    esac
                    ;;
            esac
            ;;
        5)
            case $cmd in
                agent)
                    case $subcmd in
                        update)
                            _describe -t agents 'agent' agent_names
                            ;;
                        use)
                            local agent=${words[4]}
                            local -a versions
                            if [[ -d ~/.agentbox/bin/$agent ]]; then
                                versions=(${(f)"$(command ls ~/.agentbox/bin/$agent 2>/dev/null | grep -v current)"})
                                (( ${#versions} )) && _describe -t versions 'version' versions
                            fi
                            ;;
                    esac
                    ;;
            esac
            ;;
    esac
}
compdef _agentbox agentbox
`
	base = strings.ReplaceAll(base, "{{.AgentNamesZsh}}", agentNamesZsh)
	if cmdName != "agentbox" {
		base += fmt.Sprintf("compdef _agentbox %s\n", cmdName)
	}
	return base
}

func sanitizeFuncName(name string) string {
	result := strings.ReplaceAll(name, "-", "_")
	result = strings.ReplaceAll(result, ".", "_")
	return result
}
