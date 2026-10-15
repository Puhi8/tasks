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
tasks --sync       # sync once and exit
```

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
