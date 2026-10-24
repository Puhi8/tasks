#compdef tasks
# zsh completion for tasks; load it in ~/.zshrc after compinit with: eval "$(tasks completion zsh)"

_tasks() {
	local context state state_descr line
	typeset -A opt_args
	_arguments \
		'--list[only this list]:list:->lists' \
		'--due[due date of an added task]:date:(today tomorrow +1 +7)' \
		'--config[config file]:file:_files' \
		'--version[print the version]' \
		'--help[show the help]' \
		'1:command:->commands' \
		'*:task:->tasks'
	case $state in
	commands)
		local -a commands=(
			'list:print the open tasks'
			'lists:print the list names'
			'show:print a task with its subtasks and descriptions'
			'add:add a task'
			'done:complete tasks'
			'delete:delete tasks with their subtasks'
			'sync:sync once and exit'
			'completion:print this completion script'
		)
		_describe command commands
		;;
	lists)
		local -a lists=(${(f)"$(tasks lists 2>/dev/null)"})
		compadd -a lists
		;;
	tasks)
		[[ $line[1] == (show|done|delete) ]] || return 1
		local -a ids
		local entry
		# _describe reads "value:description", so a colon inside the id is escaped
		for entry in ${(f)"$(tasks __ids 2>/dev/null)"}; do
			ids+=("${${entry%%$'\t'*}//:/\\:}:${entry#*$'\t'}")
		done
		_describe task ids
		;;
	esac
}

compdef _tasks tasks
