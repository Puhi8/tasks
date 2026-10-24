# tasks

A keyboard-driven terminal editor for your [Tasks.org](https://tasks.org) tasks. It syncs with any CalDAV server (such as Radicale) and keeps a local copy, so it also works offline.

- Changes upload automatically whenever the server is reachable.
- Everything Tasks.org stores is kept, even details this app does not show, like reminders.
- Nothing is overwritten silently: when a task changed in two places, you choose which version wins.
- Subtasks, tags with colors, repeating tasks, priorities, search and filters, mouse support.

## Install

Linux (x86-64 and ARM), using the install script attached to every release:

```sh
curl -fsSL https://github.com/puhi8/tasks/releases/latest/download/install.sh | sh
```

From a checkout (needs Go): `./install.sh --source`. To remove the program (config and tasks are kept): `install.sh --uninstall`.

## Set up

Without a config, tasks works on its own, local only. To sync, copy [config.example.toml](config.example.toml) (the install script puts it in `~/.config/tasks-tui/`) to `~/.config/tasks-tui/config.toml` and fill in your server:

```toml
[server]
url = "https://dav.example.com/"
username = "alice"
password_command = "secret-tool lookup service dav"
```

The password is never stored in the file: `password_command` prints it, for example from your keyring, or set the `TASKS_TUI_PASSWORD` environment variable.

## Use

```sh
tasks              # open your tasks
tasks --list Work  # open one list
tasks sync         # sync once and exit
```

It also works as a plain command, for scripts or a quick look:

```sh
tasks list due:today                           # filters work like the in-app search
tasks add Buy milk --list Home --due tomorrow
tasks show Website                             # a task with all subtasks and full descriptions
tasks done a1b2c3d4                            # the id from tasks list, or the whole title
```

`tasks --help` lists every command. For tab completion of commands, lists and task ids, add `eval "$(tasks completion zsh)"` to `~/.zshrc` (after `compinit`), or `eval "$(tasks completion bash)"` to `~/.bashrc`.

### AI agents

[skills/project-tasks](skills/project-tasks/SKILL.md) is a skill for Claude Code and other agents that read `SKILL.md` files. It teaches the agent to use these commands, so "look at the task MyProject" makes it read that task with all its subtasks and descriptions, do the work and mark each finished item done. Install it by copying the folder into `~/.claude/skills/`.

Press `?` in the app for all keys. The most used ones:

| Key | Action |
| --- | --- |
| `j` / `k` | move |
| `a` | add a task |
| `enter` | edit, `esc` saves |
| `x` | complete |
| `d` | delete |
| `/` | search, `#name` for a tag |
| `s` | sync |
| `q` | quit |
