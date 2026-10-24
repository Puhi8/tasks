# bash completion for tasks; load it in ~/.bashrc with: eval "$(tasks completion bash)"

_tasks() {
	local current=${COMP_WORDS[COMP_CWORD]}
	case ${COMP_WORDS[COMP_CWORD-1]} in
	--list)
		local IFS=$'\n'
		COMPREPLY=($(compgen -W "$(tasks lists 2>/dev/null)" -- "$current"))
		return
		;;
	--due | --config) return ;;
	esac
	if [[ $current == -* ]]; then
		COMPREPLY=($(compgen -W "--list --due --config --version --help" -- "$current"))
		return
	fi
	# the command is the first word that is neither a flag nor a flag's value
	local command="" index
	for ((index = 1; index < COMP_CWORD; index++)); do
		case ${COMP_WORDS[index-1]} in --list | --due | --config) continue ;; esac
		if [[ ${COMP_WORDS[index]} != -* ]]; then
			command=${COMP_WORDS[index]}
			break
		fi
	done
	case $command in
	"") COMPREPLY=($(compgen -W "list lists show add done delete sync completion" -- "$current")) ;;
	show | done | delete) COMPREPLY=($(compgen -W "$(tasks __ids 2>/dev/null | cut -f1)" -- "$current")) ;;
	esac
}

complete -F _tasks tasks
