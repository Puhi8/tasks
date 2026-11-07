---
name: project-tasks
description: Read a project's to-do list from the user's task manager with the `tasks` CLI (Tasks.org / CalDAV tasks), read-only, and work through it. Use when the user says things like "look at the task myproject", "do the tasks for X", "work through the subtasks of X", "what is left on X", or names a task and expects you to read its subtasks and descriptions and get them done. Also use when asked to list or check tasks from the terminal. Never mark a task done: the user does that.
---

# Project tasks

The user keeps project to-dos as tasks. A project is usually one task: its subtasks (nested to any depth) are the work items, and each task's description holds the details. The root task's description usually explains the project as a whole: goals, constraints, decisions.

**The task list is read-only.** Read it, do the work it describes, report back. Never complete, add, edit or delete a task yourself: changing task state is the user's job, done by a human, never automated. Even when an item is clearly finished and verified, leave it open and say so in your report.

## Commands

`tasks` works on a local copy that syncs with the user's server.

| Command | What it does |
| --- | --- |
| `tasks sync` | fetch the newest tasks, upload local changes |
| `tasks show TASK` | the task and all its subtasks as a Markdown checklist, descriptions in full |
| `tasks list [SEARCH]` | open tasks, one per line, id in the first column |
| `tasks lists` | the list names |

- TASK is the 8 character id from `show` or `list`, or the whole title (case does not matter). Prefer ids, titles are often not unique.
- SEARCH words match titles and descriptions; filters: `tag:NAME`, `due:today|overdue|soon`, `status:completed`, `list:NAME`.
- `--list NAME` limits any command to one list.
- Never run `tasks` without a command: that opens the interactive app and the shell hangs.
- The commands above are the only ones to run. `tasks done`, `tasks add` and `tasks delete` write to the list, so never run them unless the user asks for that exact change in this conversation.

`tasks show` prints, for example:

```markdown
- [ ] MyProject (id 4981c9eb)
  Static site for the shop.
  Use plain HTML, no framework.
  - [ ] Contact form (id 0a2cf027, due 2026-11-01, high priority, #web)
    Send to the address in the config.
    - [ ] Spam filter (id 671e20a2)
  - [x] Logo (id d739b06f)
```

`[x]` items are already done. Read them for context only.

## Workflow

1. Check that the CLI exists with `command -v tasks`. If it is missing, say so and stop.
2. Run `tasks sync`. "no server configured" means the tasks are local only, which is fine. On any other error, tell the user the list may be out of date and go on with the local copy.
3. Run `tasks show "<name>"`. On "no task" or "matches N tasks", run `tasks list <word>` and pick the task whose title fits the name. If several fit, ask the user which one.
4. Read the whole checklist, the descriptions included, before you change anything. The open `[ ]` items are the work. Do them in the order shown (due dates first, then priority), and do a task's subtasks before the task itself.
5. Do each item in the current project. Follow its description. If an item is too vague to act on and guessing would change what gets built, ask. Verify each item (build, tests, run it) before you call it finished.
6. When an item is finished, leave it open in the task list and move on to the next one. Do not run `tasks done`, and do not ask the user to let you run it: the user closes items themselves after reviewing the work.
7. End with a short report: the items you finished (titles and ids) so the user can mark them done, what is left and why. Call out separately the items only the user can do (buy, call, decide, log in somewhere) and anything you skipped or could not verify.
