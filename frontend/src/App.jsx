import { useEffect, useMemo, useRef, useState } from "react";
import "./App.css";
import {
  LoadState,
  OpenTerminal,
  PickAndAddProject,
  RefreshState,
  RemoveProject,
  RenameWorktree,
  SetAlwaysOnTop,
} from "../wailsjs/go/main/App";
import { EventsOff, EventsOn } from "../wailsjs/runtime/runtime";

const emptyState = {
  config: { projects: [], worktreeNames: {}, window: { alwaysOnTop: true } },
  projects: [],
  scanning: false,
  hook: { enabled: false, port: 48731 },
};

function App() {
  const [state, setState] = useState(emptyState);
  const [error, setError] = useState("");
  const [menu, setMenu] = useState(null);
  const [renameTarget, setRenameTarget] = useState(null);
  const [renameValue, setRenameValue] = useState("");
  const [busyAction, setBusyAction] = useState("");
  const renameInputRef = useRef(null);

  useEffect(() => {
    let alive = true;
    LoadState()
      .then((next) => {
        if (alive) setState(next || emptyState);
        return RefreshState();
      })
      .then((next) => {
        if (alive && next) setState(next);
      })
      .catch(showError);

    EventsOn("state:updated", (next) => {
      if (next) setState(next);
    });
    EventsOn("state:error", (message) => showError(message));

    return () => {
      alive = false;
      EventsOff("state:updated");
      EventsOff("state:error");
    };
  }, []);

  useEffect(() => {
    const close = () => setMenu(null);
    window.addEventListener("click", close);
    window.addEventListener("blur", close);
    return () => {
      window.removeEventListener("click", close);
      window.removeEventListener("blur", close);
    };
  }, []);

  useEffect(() => {
    if (renameTarget) {
      requestAnimationFrame(() => {
        renameInputRef.current?.focus();
        renameInputRef.current?.select();
      });
    }
  }, [renameTarget]);

  useEffect(() => {
    const scrollProjectStack = (event) => {
      const target = event.target instanceof Element ? event.target : null;
      const list = target?.closest(".project-stack");
      if (!list) return;

      const maxScroll = list.scrollHeight - list.clientHeight;
      if (maxScroll <= 0 || event.deltaY === 0) return;

      const delta =
        event.deltaMode === 1 ? event.deltaY * 16 : event.deltaMode === 2 ? event.deltaY * list.clientHeight : event.deltaY;
      const nextTop = Math.max(0, Math.min(maxScroll, list.scrollTop + delta));
      if (nextTop !== list.scrollTop) {
        event.preventDefault();
        list.scrollTop = nextTop;
      }
    };

    window.addEventListener("wheel", scrollProjectStack, { capture: true, passive: false });
    return () => {
      window.removeEventListener("wheel", scrollProjectStack, { capture: true });
    };
  }, []);

  const stats = useMemo(() => {
    const projects = state?.projects || [];
    const worktrees = projects.reduce((sum, project) => sum + (project.worktrees?.length || 0), 0);
    const active = projects.reduce(
      (sum, project) =>
        sum + (project.worktrees || []).filter((worktree) => worktree.activity?.state === "working").length,
      0,
    );
    const dirty = projects.reduce(
      (sum, project) => sum + (project.worktrees || []).filter((worktree) => !worktree.status?.clean).length,
      0,
    );
    return { projects: projects.length, worktrees, active, dirty };
  }, [state]);

  const run = async (label, task) => {
    setError("");
    setBusyAction(label);
    try {
      const next = await task();
      if (next) setState(next);
    } catch (err) {
      showError(err);
    } finally {
      setBusyAction("");
    }
  };

  const showError = (err) => {
    const message = err?.message || String(err || "");
    if (message) setError(message);
  };

  const addProject = () => run("add", () => PickAndAddProject());
  const refresh = () => run("refresh", () => RefreshState());
  const toggleTop = () => run("top", () => SetAlwaysOnTop(!state?.config?.window?.alwaysOnTop));
  const removeProject = (project) => run("remove", () => RemoveProject(project.id));
  const openTerminal = (worktree) => run("terminal", () => OpenTerminal(worktree.path).then(() => null));

  const openRename = (worktree) => {
    setMenu(null);
    setRenameTarget(worktree);
    setRenameValue(worktree.displayName || worktree.name || "");
  };

  const closeRename = () => {
    setRenameTarget(null);
    setRenameValue("");
  };

  const submitRename = (event) => {
    event.preventDefault();
    if (!renameTarget) return;
    const worktree = renameTarget;
    const nextName = renameValue;
    closeRename();
    run("rename", () => RenameWorktree(worktree.path, nextName));
  };

  const openMenu = (event, item) => {
    event.preventDefault();
    event.stopPropagation();
    setMenu({
      x: event.clientX,
      y: event.clientY,
      ...item,
    });
  };

  const projects = state?.projects || [];

  return (
    <div className="monitor-shell">
      <header className="monitor-head">
        <div className="title-block">
          <div className="logo" />
          <div>
            <h1>Worktrees</h1>
            <p>
              {stats.projects}P / {stats.worktrees}W
              {stats.active ? ` / ${stats.active} running` : ""}
            </p>
          </div>
        </div>
        <div className="mini-actions">
          <span className={`hook-dot ${state?.hook?.enabled ? "on" : ""}`} title={state?.hook?.error || `Hook ${state?.hook?.port || ""}`} />
          <button onClick={addProject} disabled={!!busyAction} title="添加项目">+</button>
          <button onClick={refresh} disabled={state?.scanning || busyAction === "refresh"} title="刷新">
            {state?.scanning ? "..." : "R"}
          </button>
          <button className={state?.config?.window?.alwaysOnTop ? "active" : ""} onClick={toggleTop} title="置顶">T</button>
        </div>
      </header>

      <section className="stat-strip">
        <Metric label="running" value={stats.active} tone={stats.active ? "run" : ""} />
        <Metric label="dirty" value={stats.dirty} tone={stats.dirty ? "dirty" : ""} />
        <Metric label="hook" value={state?.hook?.enabled ? "on" : "off"} tone={state?.hook?.enabled ? "run" : ""} />
      </section>

      {error ? (
        <div className="notice">
          <span>{error}</span>
          <button onClick={() => setError("")}>x</button>
        </div>
      ) : null}

      <main className="project-stack">
        {projects.length === 0 ? (
          <button className="empty-state" onClick={addProject}>选择 Git 项目目录</button>
        ) : (
          projects.map((project) => (
            <ProjectBlock
              key={project.id}
              project={project}
              onRemove={() => removeProject(project)}
              onOpenTerminal={openTerminal}
              onContextMenu={openMenu}
            />
          ))
        )}
      </main>

      {menu ? (
        <div className="context-menu" style={{ left: menu.x, top: menu.y }} onClick={(event) => event.stopPropagation()}>
          {menu.worktree ? <button onClick={() => openRename(menu.worktree)}>自定义名称</button> : null}
          {menu.project ? <button onClick={() => { setMenu(null); removeProject(menu.project); }}>移除项目</button> : null}
        </div>
      ) : null}

      {renameTarget ? (
        <div className="rename-backdrop" onClick={closeRename}>
          <form className="rename-dialog" onSubmit={submitRename} onClick={(event) => event.stopPropagation()}>
            <label htmlFor="rename-input">自定义显示名称</label>
            <input
              id="rename-input"
              ref={renameInputRef}
              value={renameValue}
              onChange={(event) => setRenameValue(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === "Escape") closeRename();
              }}
            />
            <div className="rename-actions">
              <button type="button" onClick={closeRename}>取消</button>
              <button type="submit" disabled={busyAction === "rename"}>保存</button>
            </div>
          </form>
        </div>
      ) : null}
    </div>
  );
}

function Metric({ label, value, tone }) {
  return (
    <div className={`metric ${tone || ""}`}>
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  );
}

function ProjectBlock({ project, onRemove, onOpenTerminal, onContextMenu }) {
  const worktrees = project.worktrees || [];
  const active = worktrees.filter((worktree) => worktree.activity?.state === "working").length;

  return (
    <section className={`project-block ${project.error ? "has-error" : ""}`}>
      <div className="project-head" onContextMenu={(event) => onContextMenu(event, { project })}>
        <div className="project-title">
          <h2 title={project.name}>{project.name}</h2>
          <p title={project.root}>{project.root}</p>
        </div>
        <div className="project-counts">
          {active ? <span className="active-count">{active}</span> : null}
          <span>{worktrees.length}</span>
          <button title="移除项目" onClick={onRemove}>x</button>
        </div>
      </div>

      {project.error ? <div className="project-error">{project.error}</div> : null}

      <div className="worktree-stack">
        {worktrees.map((worktree) => (
          <WorktreeRow
            key={worktree.path}
            worktree={worktree}
            onClick={() => onOpenTerminal(worktree)}
            onContextMenu={(event) => onContextMenu(event, { worktree })}
          />
        ))}
      </div>
    </section>
  );
}

function WorktreeRow({ worktree, onClick, onContextMenu }) {
  const working = worktree.activity?.state === "working";
  const finished = worktree.activity?.state === "finished";
  const errored = worktree.activity?.state === "error";
  const dirtyFiles = worktree.status?.dirtyFiles || 0;
  const statusText = worktree.status?.clean ? "clean" : `${dirtyFiles}chg`;
  const title = worktree.displayName || worktree.name;

  return (
    <button
      className={`worktree-row ${working ? "working" : ""} ${finished ? "finished" : ""} ${errored ? "errored" : ""}`}
      onClick={onClick}
      onContextMenu={onContextMenu}
      title={`${title} - ${worktree.path}`}
    >
      <span className="wave" />
      <span className={`state-dot ${working ? "hot" : ""} ${finished ? "done" : ""} ${errored ? "bad" : ""}`} />
      <span className="row-main">
        <span className="row-title">{title}</span>
        <span className="row-sub">{worktree.branch || "detached"} {worktree.head || ""}</span>
      </span>
      <span className="row-meta">
        <span className={worktree.status?.clean ? "mini-tag clean" : "mini-tag dirty"}>{statusText}</span>
        {worktree.status?.ahead ? <span className="mini-tag info">+{worktree.status.ahead}</span> : null}
        {worktree.status?.behind ? <span className="mini-tag muted">-{worktree.status.behind}</span> : null}
      </span>
    </button>
  );
}

export default App;
