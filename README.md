# Worktree Pulse

Compact always-on-top desktop monitor for Git worktrees and Codex/ClaudeCode activity.

## Run

```bash
npm start
```

The Wails dev bridge opens at `http://localhost:34115`.

## Features

- Add a Git project directory and discover all related worktrees.
- Stay visible as a compact desktop card with dense project/worktree status.
- Show active AI/tool work with green wave animation.
- Click a worktree row to open its Terminal.
- Right-click a worktree row to rename it.
- Refresh without blocking the window.
- Accept Codex, ClaudeCode, or custom tool activity through a local hook bridge.

## Hook Bridge

The app listens on:

```text
http://127.0.0.1:48731/activity
```

Any tool can mark a worktree as working:

```bash
curl -sS http://127.0.0.1:48731/activity \
  -H 'content-type: application/json' \
  -d '{"provider":"codex","raw":{"hook_event_name":"UserPromptSubmit","cwd":"/path/to/worktree"}}'
```

And mark it finished:

```bash
curl -sS http://127.0.0.1:48731/activity \
  -H 'content-type: application/json' \
  -d '{"provider":"codex","raw":{"hook_event_name":"Stop","cwd":"/path/to/worktree"}}'
```

The bridge also accepts Claude Code style payloads containing `cwd`, `workspace.project_dir`, `workspace.git_worktree`, or `worktree.path`.

## Build

```bash
npm run build
```
