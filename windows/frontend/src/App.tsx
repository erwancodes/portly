import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { allServers, bridge, keepMetrics, inShell, isLive, openURL, parse, projectColor, record, serverKey, type History, type LogsResponse, type PortlyStatus, type ProjectStatus, type ServerStatus, type Verb } from "./lib";
import { Dot, Icon, Kbd, Logo } from "./ui";
import { OverviewView, ResourcesView, ServerView, SettingsView, TemporaryView } from "./views";

export type View = "overview" | "server" | "temporary" | "resources" | "settings";

export function App() {
  const [view, setView] = useState<View>("overview");
  const [status, setStatus] = useState<PortlyStatus | null>(null);
  const [history, setHistory] = useState<History>({});
  const [selectedKey, setSelectedKey] = useState("");
  const [logs, setLogs] = useState<string[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [pending, setPending] = useState("");
  const [palette, setPalette] = useState(false);
  const [updatedAt, setUpdatedAt] = useState(0);

  const servers = useMemo(() => allServers(status), [status]);
  const selected = servers.find((server) => serverKey(server) === selectedKey);

  const lastStatus = useRef<PortlyStatus | null>(null);
  const refresh = useCallback(async () => {
    try {
      const next = keepMetrics(lastStatus.current, parse<PortlyStatus>(await bridge().GetStatus()));
      lastStatus.current = next;
      setStatus(next);
      setHistory((previous) => record(previous, next));
      setUpdatedAt(Date.now());
      setError("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not reach Portly.");
    } finally {
      setLoading(false);
    }
  }, []);

  const refreshLogs = useCallback(async () => {
    if (!selectedKey || view !== "server") return;
    try {
      setLogs(parse<LogsResponse>(await bridge().GetLogs(selectedKey, 800)).lines);
    } catch {
      /* Keep the last useful scrollback. */
    }
  }, [selectedKey, view]);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 2000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  useEffect(() => {
    setLogs([]);
    void refreshLogs();
    const timer = window.setInterval(() => void refreshLogs(), 1200);
    return () => window.clearInterval(timer);
  }, [refreshLogs]);

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPalette((open) => !open);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const openServer = (key: string) => {
    setSelectedKey(key);
    setView("server");
  };

  const runAction = async (verb: Verb, target: string) => {
    setPending(`${verb}:${target}`);
    try {
      await bridge().Action(verb, target);
      await refresh();
      await refreshLogs();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : `Could not ${verb} ${target}.`);
    } finally {
      setPending("");
    }
  };

  const writeTerminal = async (data: string) => {
    if (!selectedKey || !data) return;
    try {
      await bridge().WriteTerminal(selectedKey, data);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Could not write to the terminal.");
    }
  };

  return (
    <div className="shell">
      <TitleBar error={error} onPalette={() => setPalette(true)} />
      <div className="body">
        <Sidebar
          view={view}
          status={status}
          loading={loading}
          selectedKey={view === "server" ? selectedKey : ""}
          temporaryCount={status?.temporaryServers.filter((server) => isLive(server.state)).length ?? 0}
          onView={setView}
          onServer={openServer}
          version={status?.version}
          error={error}
        />
        <main className="main" key={view === "server" ? `server:${selectedKey}` : view}>
          {error && (
            <div className="alert rise" role="alert">
              <div>
                <strong>Portly is unreachable</strong>
                <span>{error}</span>
              </div>
              <button className="btn" onClick={() => void refresh()}>Try again</button>
            </div>
          )}
          {view === "overview" && <OverviewView status={status} loading={loading} history={history} updatedAt={updatedAt} pending={pending} onServer={openServer} onAction={runAction} />}
          {view === "server" && selected && <ServerView server={selected} history={history} logs={logs} pending={pending} onAction={runAction} onWrite={writeTerminal} onBack={() => setView("overview")} />}
          {view === "server" && !selected && !loading && <div className="empty rise"><h1>This server is gone</h1><p>It was removed from Portly. Pick another one in the sidebar.</p></div>}
          {view === "temporary" && <TemporaryView servers={status?.temporaryServers ?? []} onServer={openServer} />}
          {view === "resources" && <ResourcesView status={status} history={history} onServer={openServer} />}
          {view === "settings" && <SettingsView status={status} onQuit={() => void bridge().Quit()} onUpdates={() => bridge().OpenReleases()} />}
        </main>
      </div>
      {palette && <Palette servers={servers} onClose={() => setPalette(false)} onServer={openServer} onView={setView} onAction={runAction} />}
    </div>
  );
}

function TitleBar({ error, onPalette }: { error: string; onPalette: () => void }) {
  const runtime = window.runtime;
  return (
    <header className="titlebar" onDoubleClick={(event) => { if (event.target === event.currentTarget) runtime?.WindowToggleMaximise(); }}>
      <div className="titlebar-brand">
        <Logo />
        <span>Portly</span>
        <span className={`daemon ${error ? "is-down" : ""}`}>{error ? "Daemon offline" : "Supervising"}</span>
      </div>
      <button className="search-trigger" onClick={onPalette}>
        <Icon name="search" size={14} />
        <span>Jump to a server or action</span>
        <Kbd>Ctrl K</Kbd>
      </button>
      <div className="titlebar-end">
        {inShell && runtime && (
          <div className="window-controls">
            <button aria-label="Minimize" onClick={() => runtime.WindowMinimise()}>
              <svg width="10" height="10" viewBox="0 0 10 10"><path d="M0 5h10" stroke="currentColor" /></svg>
            </button>
            <button aria-label="Maximize" onClick={() => runtime.WindowToggleMaximise()}>
              <svg width="10" height="10" viewBox="0 0 10 10"><rect x="0.5" y="0.5" width="9" height="9" fill="none" stroke="currentColor" /></svg>
            </button>
            <button aria-label="Hide to tray" className="is-close" onClick={() => runtime.WindowHide()}>
              <svg width="10" height="10" viewBox="0 0 10 10"><path d="M0 0l10 10M10 0L0 10" stroke="currentColor" /></svg>
            </button>
          </div>
        )}
      </div>
    </header>
  );
}

const NAV: { id: View; label: string; icon: string }[] = [
  { id: "overview", label: "Servers", icon: "home" },
  { id: "temporary", label: "Temporary jobs", icon: "clock" },
  { id: "resources", label: "Resources", icon: "chart" },
  { id: "settings", label: "Settings", icon: "gear" },
];

function Sidebar({ view, status, loading, selectedKey, temporaryCount, onView, onServer, version, error }: { view: View; status: PortlyStatus | null; loading: boolean; selectedKey: string; temporaryCount: number; onView: (view: View) => void; onServer: (key: string) => void; version?: string; error: string }) {
  const [filter, setFilter] = useState("");
  const projects = (status?.projects ?? []).filter((project) => {
    const needle = filter.trim().toLowerCase();
    return !needle || project.name.toLowerCase().includes(needle) || project.servers.some((server) => server.name.toLowerCase().includes(needle) || String(server.port ?? "").includes(needle));
  });
  const running = status ? status.projects.flatMap((project) => project.servers).filter((server) => isLive(server.state)).length : 0;
  return (
    <nav className="sidebar" aria-label="Portly">
      <div className="nav">
        {NAV.map((item) => (
          <button key={item.id} className={`nav-item ${view === item.id ? "is-active" : ""}`} aria-current={view === item.id ? "page" : undefined} onClick={() => onView(item.id)}>
            <Icon name={item.icon} />
            <span>{item.label}</span>
            {item.id === "overview" && running > 0 && <b className="num">{running}</b>}
            {item.id === "temporary" && temporaryCount > 0 && <b className="num">{temporaryCount}</b>}
          </button>
        ))}
      </div>
      <label className="filter">
        <Icon name="search" size={14} />
        <input value={filter} onChange={(event) => setFilter(event.target.value)} placeholder="Filter projects" aria-label="Filter projects" />
      </label>
      <div className="tree">
        {loading && !status && [70, 52, 64, 45].map((width, index) => <span key={index} className="tree-skeleton" style={{ width: `${width}%` }} />)}
        {projects.map((project) => <TreeProject key={project.id} project={project} selectedKey={selectedKey} onServer={onServer} />)}
        {status && !projects.length && <p className="tree-empty">{filter ? "No project matches." : "No projects yet. Add one with portly add-project."}</p>}
      </div>
      <div className="sidebar-foot">
        <span className={`foot-dot ${error ? "is-down" : ""}`} />
        <span>{error ? "Daemon offline" : `${status?.projects.length ?? 0} projects`}</span>
        <span className="num">v{version ?? "…"}</span>
      </div>
    </nav>
  );
}

function TreeProject({ project, selectedKey, onServer }: { project: ProjectStatus; selectedKey: string; onServer: (key: string) => void }) {
  const live = project.servers.some((server) => isLive(server.state));
  return (
    <section className={`tree-project ${live ? "is-live" : ""}`}>
      <div className="tree-head">
        <span className="swatch" style={{ background: projectColor(project.color) }} />
        <span>{project.name}</span>
      </div>
      {project.servers.map((server) => {
        const key = serverKey(server);
        return (
          <button key={server.id} className={`tree-server ${selectedKey === key ? "is-selected" : ""}`} onClick={() => onServer(key)}>
            <Dot state={server.state} />
            <span>{server.name}</span>
            <span className="num">{server.port ?? ""}</span>
          </button>
        );
      })}
    </section>
  );
}

type Command = { id: string; label: string; hint: string; group: string; icon: string; state?: ServerStatus["state"]; run: () => void };

function Palette({ servers, onClose, onServer, onView, onAction }: { servers: ServerStatus[]; onClose: () => void; onServer: (key: string) => void; onView: (view: View) => void; onAction: (verb: Verb, target: string) => void }) {
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const listRef = useRef<HTMLDivElement>(null);

  const commands = useMemo<Command[]>(() => {
    const list: Command[] = [];
    for (const server of servers) {
      const key = serverKey(server);
      list.push({ id: `go:${key}`, label: `${server.projectName} / ${server.name}`, hint: server.port ? `:${server.port}` : server.command, group: "Servers", icon: "chevron", state: server.state, run: () => onServer(key) });
    }
    for (const server of servers) {
      const key = serverKey(server);
      if (isLive(server.state)) {
        list.push({ id: `restart:${key}`, label: `Restart ${key}`, hint: "Action", group: "Actions", icon: "restart", run: () => onAction("restart", key) });
        list.push({ id: `stop:${key}`, label: `Stop ${key}`, hint: "Action", group: "Actions", icon: "stop", run: () => onAction("stop", key) });
        if (server.url) list.push({ id: `open:${key}`, label: `Open ${key} in the browser`, hint: server.url.replace(/^https?:\/\//, ""), group: "Actions", icon: "external", run: () => openURL(server.url!) });
      } else {
        list.push({ id: `start:${key}`, label: `Start ${key}`, hint: "Action", group: "Actions", icon: "play", run: () => onAction("start", key) });
      }
    }
    for (const item of NAV) list.push({ id: `view:${item.id}`, label: item.label, hint: "Go to", group: "Pages", icon: item.icon, run: () => onView(item.id) });
    return list;
  }, [servers, onAction, onServer, onView]);

  const results = useMemo(() => {
    const words = query.toLowerCase().split(/\s+/).filter(Boolean);
    const matched = commands.filter((command) => words.every((word) => `${command.label} ${command.hint}`.toLowerCase().includes(word)));
    // Without a query the list stays short: the servers that are up first, then pages.
    if (!words.length) return [...matched.filter((command) => command.group === "Servers" && command.state && isLive(command.state)).slice(0, 8), ...matched.filter((command) => command.group === "Pages")];
    return matched.slice(0, 40);
  }, [commands, query]);

  useEffect(() => setIndex(0), [query]);
  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>(`[data-index="${index}"]`)?.scrollIntoView({ block: "nearest" });
  }, [index]);

  const choose = (command?: Command) => {
    if (!command) return;
    command.run();
    onClose();
  };

  return (
    <div className="palette-backdrop" onMouseDown={onClose}>
      <div className="palette" role="dialog" aria-label="Command palette" onMouseDown={(event) => event.stopPropagation()}>
        <label className="palette-input">
          <Icon name="search" />
          <input
            autoFocus
            value={query}
            placeholder="Type a project, a port or an action"
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Escape") onClose();
              if (event.key === "ArrowDown") { event.preventDefault(); setIndex((value) => Math.min(results.length - 1, value + 1)); }
              if (event.key === "ArrowUp") { event.preventDefault(); setIndex((value) => Math.max(0, value - 1)); }
              if (event.key === "Enter") { event.preventDefault(); choose(results[index]); }
            }}
          />
        </label>
        <div className="palette-list" ref={listRef}>
          {results.map((command, position) => (
            <div key={command.id}>
              {(position === 0 || results[position - 1].group !== command.group) && <p className="palette-group">{command.group}</p>}
              <button data-index={position} className={`palette-item ${position === index ? "is-active" : ""}`} onMouseMove={() => setIndex(position)} onClick={() => choose(command)}>
                {command.state ? <Dot state={command.state} /> : <Icon name={command.icon} size={14} />}
                <span className="palette-label">{command.label}</span>
                <span className="num">{command.hint}</span>
              </button>
            </div>
          ))}
          {!results.length && <p className="palette-empty">Nothing matches “{query}”.</p>}
        </div>
        <div className="palette-foot">
          <span><Kbd>↑</Kbd><Kbd>↓</Kbd> move</span>
          <span><Kbd>Enter</Kbd> run</span>
          <span><Kbd>Esc</Kbd> close</span>
        </div>
      </div>
    </div>
  );
}
