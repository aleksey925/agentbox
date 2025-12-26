package cli

import (
	"fmt"
	"os"
	"strings"
)

func (a *App) cmdCompletions(args []string) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: agentbox completions <shell> [command-name]\n")
		fmt.Fprintf(os.Stderr, "Supported shells: bash, zsh\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  agentbox completions bash\n")
		fmt.Fprintf(os.Stderr, "  agentbox completions zsh abox  # for alias 'abox'\n")
		return 1
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
	tmpl := `_{{.FuncName}}() {
    local cur prev commands agents_sub agent_names run_flags
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    commands="init run agents clean help completions upgrade version"
    agents_sub="update use"
    agent_names="claude copilot codex gemini"
    run_flags="--build --build-no-cache"

    case "$prev" in
        {{.CmdName}})
            COMPREPLY=($(compgen -W "$commands" -- "$cur"))
            ;;
        run)
            COMPREPLY=($(compgen -W "$run_flags" -- "$cur"))
            ;;
        agents)
            COMPREPLY=($(compgen -W "$agents_sub" -- "$cur"))
            ;;
        update)
            COMPREPLY=($(compgen -W "$agent_names --all -a" -- "$cur"))
            ;;
        use)
            COMPREPLY=($(compgen -W "$agent_names" -- "$cur"))
            ;;
        claude|copilot|codex|gemini)
            if [[ "${COMP_WORDS[COMP_CWORD-2]}" == "use" ]]; then
                local versions=$(ls ~/.agentbox/bin/"$prev"/ 2>/dev/null | grep -v current)
                COMPREPLY=($(compgen -W "$versions" -- "$cur"))
            fi
            ;;
        completions)
            COMPREPLY=($(compgen -W "bash zsh" -- "$cur"))
            ;;
    esac
}
complete -F _{{.FuncName}} {{.CmdName}}
`
	funcName := "_" + sanitizeFuncName(cmdName)
	result := strings.ReplaceAll(tmpl, "{{.FuncName}}", funcName)
	result = strings.ReplaceAll(result, "{{.CmdName}}", cmdName)
	return result
}

func generateZshCompletion(cmdName string) string {
	base := `_agentbox() {
    local -a commands agents_cmds agent_names shells run_flags

    commands=(
        'init:Initialize sandbox in current directory'
        'run:Start the container'
        'agents:Manage AI agents'
        'clean:Remove sandbox files from project'
        'completions:Output shell completions'
        'upgrade:Update skeleton files from embedded'
        'help:Show help'
        'version:Show version'
    )

    run_flags=(
        '--build:Rebuild image before running'
        '--build-no-cache:Rebuild image without Docker cache'
    )

    agents_cmds=(
        'update:Update agents to latest version'
        'use:Switch agent to specific version'
    )

    agent_names=(
        'claude:Claude Code by Anthropic'
        'copilot:GitHub Copilot'
        'codex:OpenAI Codex'
        'gemini:Google Gemini'
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
                agents)
                    _describe -t commands 'agents command' agents_cmds
                    ;;
                completions)
                    _describe -t shells 'shell' shells
                    ;;
            esac
            ;;
        4)
            case $cmd in
                agents)
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
                agents)
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
