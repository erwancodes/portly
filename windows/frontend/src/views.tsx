import { useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties, type ReactNode } from "react";
import { bridge, formatBytes, isLive, openURL, projectColor, serverKey, since, splitBytes, stateLabel, TOTAL, type History, type PortlyStatus, type ProjectStatus, type ServerStatus, type Verb } from "./lib";
import { Dot, Icon, Sparkline, StatePill, useTween } from "./ui";

const delay = (ms: number): CSSProperties => ({ animationDelay: `${ms}ms` });

/** Re-render once a second so durations tick. */
function useNow(interval = 1000) {
  const [now, setNow] = useState(Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), interval);
    return () => window.clearInterval(id);
  }, [interval]);
  return now;
}

function Header({ title, children, aside }: { title: ReactNode; children?: ReactNode; aside?: ReactNode }) {
  return (
    <div className="page-head rise">
      <div>
        <h1>{title}</h1>
        {children && <p>{children}</p>}
      </div>
      {aside && <div className="page-aside">{aside}</div>}
    </div>
  );
}

function Bytes({ value, className = "" }: { value: number; className?: string }) {
  const shown = useTween(value);
  const [number, unit] = splitBytes(shown);
  return (
    <span className={`big num ${className}`}>
      {number}
      <small>{unit}</small>
    </span>
  );
}

function Percent({ value }: { value: number }) {
  const shown = useTween(value);
  return (
    <span className="big num">
      {shown.toFixed(shown >= 100 ? 0 : 1)}
      <small>%</small>
    </span>
  );
}

/* ------------------------------------------------------------------ Overview */

type Filter = "all" | "live" | "stopped";

export function OverviewView({ status, loading, history, updatedAt, pending, onServer, onAction }: { status: PortlyStatus | null; loading: boolean; history: History; updatedAt: number; pending: string; onServer: (key: string) => void; onAction: (verb: Verb, target: string) => void }) {
  const [filter, setFilter] = useState<Filter>("all");
  const now = useNow();
  if (loading && !status) return <Loading />;
  if (!status) return null;

  const servers = status.projects.flatMap((project) => project.servers);
  const live = servers.filter((server) => isLive(server.state));
  const failed = servers.filter((server) => server.state === "failed");
  const total = history[TOTAL] ?? { memory: [], cpu: [] };
  const memory = total.memory[total.memory.length - 1] ?? 0;
  const cpu = total.cpu[total.cpu.length - 1] ?? 0;

  const projects = status.projects
    .map((project) => ({ ...project, servers: project.servers.filter((server) => (filter === "all" ? true : filter === "live" ? isLive(server.state) : !isLive(server.state))) }))
    .filter((project) => project.servers.length);

  if (!servers.length) {
    return (
      <div className="empty rise">
        <h1>Nothing supervised yet</h1>
        <p>Register a project and its dev server from any terminal. It shows up here within two seconds.</p>
        <pre><code>portly add-project --name demo --path C:\path\to\project{"\n"}portly add-server demo/dev --command "npm run dev" --port 3000</code></pre>
      </div>
    );
  }

  const ago = updatedAt ? Math.max(0, Math.round((now - updatedAt) / 1000)) : 0;

  return (
    <div className="page">
      <Header title="Servers" aside={<span className="fresh"><span className="fresh-dot" />{ago <= 2 ? "Live" : `Updated ${ago}s ago`}</span>}>
        {live.length} of {servers.length} running across {status.projects.length} projects{failed.length ? `, ${failed.length} failed` : ""}.
      </Header>

      <div className="stats">
        <section className="card rise" style={delay(40)}>
          <h2>Running now</h2>
          <span className="big num">{live.length}<small>/ {servers.length}</small></span>
          <div className="port-strip" aria-hidden>
            {servers.map((server) => <span key={server.id} className={`port-tick tick-${server.state}`} title={`${serverKey(server)} · ${stateLabel(server.state)}`} />)}
          </div>
        </section>
        <section className="card rise" style={delay(90)}>
          <h2>Managed memory</h2>
          <Bytes value={memory} />
          <Sparkline values={total.memory} />
        </section>
        <section className="card rise" style={delay(140)}>
          <h2>CPU</h2>
          <Percent value={cpu} />
          <Sparkline values={total.cpu} color="var(--cpu)" max={10} />
        </section>
      </div>

      <div className="toolbar rise" style={delay(180)}>
        <div className="segmented" role="tablist" aria-label="Filter servers">
          {([["all", `All ${servers.length}`], ["live", `Running ${live.length}`], ["stopped", `Stopped ${servers.length - live.length}`]] as const).map(([id, label]) => (
            <button key={id} role="tab" aria-selected={filter === id} className={filter === id ? "is-active" : ""} onClick={() => setFilter(id)}>{label}</button>
          ))}
        </div>
      </div>

      <div className="projects">
        {projects.map((project, index) => <ProjectCard key={project.id} project={project} history={history} pending={pending} style={delay(Math.min(220 + index * 35, 600))} onServer={onServer} onAction={onAction} />)}
        {!projects.length && <p className="muted">No server in this state.</p>}
      </div>
    </div>
  );
}

function ProjectCard({ project, history, pending, style, onServer, onAction }: { project: ProjectStatus; history: History; pending: string; style: CSSProperties; onServer: (key: string) => void; onAction: (verb: Verb, target: string) => void }) {
  const color = projectColor(project.color);
  return (
    <section className="project rise" style={{ ...style, ["--project" as string]: color }}>
      <header className="project-head">
        <span className="swatch" style={{ background: color }} />
        <h2>{project.name}</h2>
        <span className="project-root" title={project.root}>{project.root}</span>
      </header>
      {project.servers.map((server) => {
        const key = serverKey(server);
        const live = isLive(server.state);
        const busy = pending.endsWith(`:${key}`);
        return (
          <div key={server.id} className={`row ${live ? "is-live" : ""}`} role="button" tabIndex={0} onClick={() => onServer(key)} onKeyDown={(event) => { if (event.key === "Enter") onServer(key); }}>
            <Dot state={server.state} />
            <div className="row-name">
              <strong>{server.name}</strong>
              <span>{server.command}</span>
            </div>
            <div className="row-spark">{live && <Sparkline values={history[key]?.memory ?? []} height={24} color={color} />}</div>
            <span className="row-mem num">{live ? formatBytes(server.memoryBytes) : stateLabel(server.state)}</span>
            <span className="row-port num">{server.port ? `:${server.port}` : "—"}</span>
            <div className="row-actions" onClick={(event) => event.stopPropagation()}>
              {live ? (
                <>
                  {server.url && <button className="icon-btn" title={`Open ${server.url}`} onClick={() => openURL(server.url!)}><Icon name="external" size={14} /></button>}
                  <button className="icon-btn" title="Restart" disabled={busy} onClick={() => onAction("restart", key)}><Icon name="restart" size={14} /></button>
                  <button className="icon-btn" title="Stop" disabled={busy} onClick={() => onAction("stop", key)}><Icon name="stop" size={14} /></button>
                </>
              ) : (
                <button className="icon-btn is-go" title="Start" disabled={busy} onClick={() => onAction("start", key)}><Icon name="play" size={14} /></button>
              )}
            </div>
          </div>
        );
      })}
    </section>
  );
}

function Loading() {
  return (
    <div className="page">
      <div className="skeleton" style={{ width: 180, height: 26 }} />
      <div className="stats">{[0, 1, 2].map((index) => <div key={index} className="card skeleton-card" />)}</div>
      <div className="card skeleton-card tall" />
    </div>
  );
}

/* -------------------------------------------------------------------- Server */

export function ServerView({ server, history, logs, pending, onAction, onWrite, onBack }: { server: ServerStatus; history: History; logs: string[]; pending: string; onAction: (verb: Verb, target: string) => void; onWrite: (data: string) => void; onBack: () => void }) {
  useNow();
  const key = serverKey(server);
  const live = isLive(server.state);
  const busy = pending.endsWith(`:${key}`);
  const series = history[key] ?? { memory: [], cpu: [] };
  const [copied, setCopied] = useState(false);

  const copyDirectory = () => {
    void navigator.clipboard?.writeText(server.directory);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1400);
  };

  return (
    <div className="page server-page">
      <button className="back rise" onClick={onBack}><Icon name="back" size={14} />{server.temporary ? "Temporary jobs" : "Servers"}</button>
      <div className="page-head rise">
        <div>
          <h1>{server.projectName} <span className="slash">/</span> {server.name}</h1>
          <code className="command">{server.command}</code>
        </div>
        <div className="page-aside">
          {live ? (
            <>
              {server.url && <button className="btn btn-primary" onClick={() => openURL(server.url!)}><Icon name="external" size={14} />Open {server.port ? `localhost:${server.port}` : "in browser"}</button>}
              <button className="btn" disabled={busy} onClick={() => onAction("restart", key)}><Icon name="restart" size={14} />Restart</button>
              <button className="btn" disabled={busy} onClick={() => onAction("stop", key)}><Icon name="stop" size={14} />Stop</button>
            </>
          ) : (
            <button className="btn btn-primary" disabled={busy} onClick={() => onAction("start", key)}><Icon name="play" size={14} />Start {server.name}</button>
          )}
        </div>
      </div>

      <div className="stats four">
        <section className="card rise" style={delay(40)}>
          <h2>Status</h2>
          <StatePill state={server.state} />
          <p className="card-note">
            {live && server.startedAt ? `Up for ${since(server.startedAt)}` : server.lastExitCode !== null ? `Exited with code ${server.lastExitCode}` : "Not started in this session"}
            {server.restartCount ? ` · ${server.restartCount} restart${server.restartCount > 1 ? "s" : ""}` : ""}
          </p>
        </section>
        <section className="card rise" style={delay(80)}>
          <h2>Port</h2>
          {server.port ? (
            <button className="port-link big num" disabled={!server.url} onClick={() => server.url && openURL(server.url)}>{server.port}</button>
          ) : (
            <span className="big muted-big">None</span>
          )}
          <p className="card-note">{server.port ? (server.healthy ? "Answering on localhost" : live ? "Waiting for the port to answer" : "Free while stopped") : "This process does not listen"}</p>
        </section>
        <section className="card rise" style={delay(120)}>
          <h2>Memory</h2>
          <Bytes value={live ? server.memoryBytes ?? 0 : 0} />
          <Sparkline values={series.memory} />
        </section>
        <section className="card rise" style={delay(160)}>
          <h2>CPU</h2>
          <Percent value={live ? server.cpuPercent ?? 0 : 0} />
          <Sparkline values={series.cpu} color="var(--cpu)" max={10} />
        </section>
      </div>

      <div className="server-grid">
        <Terminal key={key} serverKey={key} name={server.name} logs={logs} live={live} onWrite={onWrite} />
        <aside className="card details rise" style={delay(240)}>
          <Detail label="Directory">
            <button className="path" onClick={copyDirectory} title="Copy the path">
              <span>{server.directory}</span>
              <Icon name={copied ? "enter" : "copy"} size={13} />
            </button>
          </Detail>
          <Detail label="Process tree">{server.processCount ? `${server.processCount} processes` : "No process"}</Detail>
          <Detail label="PID"><span className="num">{server.pid ?? "None"}</span></Detail>
          <Detail label="Started">{server.startedAt ? new Date(server.startedAt).toLocaleString() : "Never in this session"}</Detail>
          <Detail label="Resident memory"><span className="num">{server.residentMemoryBytes ? formatBytes(server.residentMemoryBytes) : "Not sampled"}</span></Detail>
          {server.lastError && <div className="error-note">{server.lastError}</div>}
        </aside>
      </div>
    </div>
  );
}

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="detail">
      <span>{label}</span>
      <div>{children}</div>
    </div>
  );
}

function lineTone(line: string) {
  if (/\[(warn|warning)\]/i.test(line)) return "is-warn";
  if (/\b(error|failed|exception|fatal|panic|ERR!)\b/i.test(line)) return "is-error";
  if (/\b(warn|warning|deprecated)\b/i.test(line)) return "is-warn";
  if (/\b(ready|compiled|listening|started|local:)\b/i.test(line) || /https?:\/\/(localhost|127\.0\.0\.1)/.test(line)) return "is-good";
  return "";
}

function highlight(line: string, query: string): ReactNode {
  if (!query) return line || " ";
  const parts = line.split(new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`, "ig"));
  return parts.map((part, index) => (index % 2 ? <mark key={index}>{part}</mark> : part));
}

function Terminal({ serverKey: key, name, logs, live, onWrite }: { serverKey: string; name: string; logs: string[]; live: boolean; onWrite: (data: string) => void }) {
  const [query, setQuery] = useState("");
  const [input, setInput] = useState("");
  const [follow, setFollow] = useState(true);
  const [copied, setCopied] = useState(false);
  const outputRef = useRef<HTMLDivElement>(null);
  const shown = useMemo(() => {
    if (query) return logs.filter((line) => line.toLowerCase().includes(query.toLowerCase()));
    // Dev servers pad their output with blank lines; keep one at most so the panel stays dense.
    return logs.filter((line, index) => line.trim() || (index > 0 && logs[index - 1].trim()));
  }, [logs, query]);

  useLayoutEffect(() => {
    if (follow && outputRef.current) outputRef.current.scrollTop = outputRef.current.scrollHeight;
  }, [shown, follow]);

  useEffect(() => {
    const element = outputRef.current;
    if (!element) return;
    const resize = () => {
      const width = Math.max(40, Math.min(240, Math.floor((element.clientWidth - 32) / 7.4)));
      const height = Math.max(12, Math.min(80, Math.floor(element.clientHeight / 20)));
      void bridge().ResizeTerminal(key, width, height).catch(() => undefined);
    };
    resize();
    const observer = new ResizeObserver(resize);
    observer.observe(element);
    return () => observer.disconnect();
  }, [key]);

  const send = () => {
    onWrite(`${input}\r\n`);
    setInput("");
  };

  return (
    <section className="card terminal rise" style={delay(200)}>
      <header className="terminal-head">
        <h2>Output</h2>
        <span className="muted num">{query ? `${shown.length} of ${logs.length} lines` : `${logs.length} lines`}</span>
        <label className="terminal-search">
          <Icon name="search" size={13} />
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search output" aria-label="Search output" />
        </label>
        <button className={`icon-btn ${follow ? "is-on" : ""}`} title={follow ? "Following new output" : "Follow new output"} onClick={() => setFollow(!follow)}><Icon name={follow ? "follow" : "pause"} size={14} /></button>
        <button className="icon-btn" title="Copy the output" onClick={() => { void navigator.clipboard?.writeText(shown.join("\n")); setCopied(true); window.setTimeout(() => setCopied(false), 1400); }}><Icon name={copied ? "enter" : "copy"} size={14} /></button>
      </header>
      <div
        className="terminal-output"
        ref={outputRef}
        onScroll={(event) => {
          const element = event.currentTarget;
          const atBottom = element.scrollHeight - element.scrollTop - element.clientHeight < 24;
          if (atBottom !== follow) setFollow(atBottom);
        }}
      >
        {shown.length ? (
          <pre><code>{shown.map((line, index) => <span key={index} className={`line ${lineTone(line)}`}>{highlight(line, query)}{"\n"}</span>)}</code></pre>
        ) : (
          <p className="terminal-empty">{query ? "No line matches." : live ? `Waiting for ${name} to print something.` : `${name} is stopped. Its next run prints here.`}</p>
        )}
      </div>
      {!follow && shown.length > 0 && <button className="jump" onClick={() => setFollow(true)}><Icon name="follow" size={13} />Jump to latest</button>}
      <div className="terminal-input">
        <span className="prompt" aria-hidden>›</span>
        <input
          value={input}
          disabled={!live}
          onChange={(event) => setInput(event.target.value)}
          onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey) { event.preventDefault(); send(); } }}
          placeholder={live ? `Send input to ${name}` : "Start the server to send input"}
          aria-label="Terminal input"
        />
        <button className="btn btn-quiet" disabled={!live || !input} onClick={send}>Send</button>
        <button className="btn btn-quiet" disabled={!live} title="Send Ctrl+C" onClick={() => onWrite("\u0003")}>Ctrl C</button>
      </div>
    </section>
  );
}

/* ----------------------------------------------------------------- Temporary */

export function TemporaryView({ servers, onServer }: { servers: ServerStatus[]; onServer: (key: string) => void }) {
  useNow();
  const ordered = [...servers].sort((a, b) => Date.parse(b.startedAt ?? "0") - Date.parse(a.startedAt ?? "0"));
  return (
    <div className="page">
      <Header title="Temporary jobs">Builds, tests and previews started with portly temp. Each one is killed with its whole process tree when it times out.</Header>
      {ordered.length ? (
        <section className="card list rise" style={delay(60)}>
          {ordered.map((server) => (
            <button key={server.id} className="job" onClick={() => onServer(serverKey(server))}>
              <Dot state={server.state} />
              <div className="row-name">
                <strong>{server.name}</strong>
                <span>{server.command}</span>
              </div>
              <span className="muted num">{server.startedAt ? since(server.startedAt, isLive(server.state) ? null : server.finishedAt) : ""}</span>
              <span className={`job-state state-${server.state}`}>{isLive(server.state) ? stateLabel(server.state) : server.lastExitCode === null ? stateLabel(server.state) : server.lastExitCode === 0 ? "Exited cleanly" : `Exit code ${server.lastExitCode}`}</span>
              <Icon name="chevron" size={14} />
            </button>
          ))}
        </section>
      ) : (
        <div className="empty rise" style={delay(60)}>
          <h1>No job running</h1>
          <p>Bounded work goes through a temporary job, so a hung build can never outlive its timeout.</p>
          <pre><code>portly temp "pnpm build" --path . --timeout 30m{"\n"}portly wait &lt;job-id&gt;</code></pre>
        </div>
      )}
    </div>
  );
}

/* ----------------------------------------------------------------- Resources */

export function ResourcesView({ status, history, onServer }: { status: PortlyStatus | null; history: History; onServer: (key: string) => void }) {
  const total = history[TOTAL] ?? { memory: [], cpu: [] };
  const colorOf = new Map((status?.projects ?? []).map((project) => [project.name, projectColor(project.color)]));
  const servers = [...(status?.projects.flatMap((project) => project.servers) ?? []), ...(status?.temporaryServers ?? [])].filter((server) => isLive(server.state));
  const ranked = servers.sort((a, b) => (b.memoryBytes ?? 0) - (a.memoryBytes ?? 0));
  const top = ranked[0]?.memoryBytes ?? 1;
  const memory = total.memory[total.memory.length - 1] ?? 0;
  const peak = Math.max(0, ...total.memory);
  const cpu = total.cpu[total.cpu.length - 1] ?? 0;
  const processes = ranked.reduce((sum, server) => sum + (server.processCount ?? 0), 0);

  return (
    <div className="page">
      <Header title="Resources">What the supervised process trees use right now, sampled every two seconds while this window is open.</Header>
      <div className="resource-top">
        <section className="card chart-card rise" style={delay(40)}>
          <div className="chart-head">
            <div>
              <h2>Managed memory</h2>
              <Bytes value={memory} />
            </div>
            <dl className="chart-facts">
              <div><dt>Peak</dt><dd className="num">{formatBytes(peak)}</dd></div>
              <div><dt>Processes</dt><dd className="num">{processes}</dd></div>
            </dl>
          </div>
          <Sparkline values={total.memory} height={150} className="chart" />
        </section>
        <section className="card chart-card rise" style={delay(90)}>
          <div className="chart-head">
            <div>
              <h2>CPU</h2>
              <Percent value={cpu} />
            </div>
          </div>
          <Sparkline values={total.cpu} height={150} color="var(--cpu)" max={10} className="chart" />
        </section>
      </div>
      <section className="card list rise" style={delay(140)}>
        <h2 className="list-title">By server</h2>
        {ranked.length ? ranked.map((server) => {
          const color = colorOf.get(server.projectName) ?? "var(--live)";
          return (
            <button key={server.id} className="usage" onClick={() => onServer(serverKey(server))}>
              <span className="usage-name"><span className="swatch" style={{ background: color }} />{server.projectName} <span className="slash">/</span> {server.name}</span>
              <span className="usage-bar"><span style={{ width: `${Math.max(2, ((server.memoryBytes ?? 0) / top) * 100)}%`, background: color }} /></span>
              <span className="num">{formatBytes(server.memoryBytes)}</span>
              <span className="num muted">{(server.cpuPercent ?? 0).toFixed(1)}%</span>
            </button>
          );
        }) : <p className="muted pad">Nothing is running, so nothing uses memory.</p>}
      </section>
    </div>
  );
}

/* ------------------------------------------------------------------ Settings */

export function SettingsView({ status, onQuit, onUpdates }: { status: PortlyStatus | null; onQuit: () => void; onUpdates: () => void }) {
  const [confirm, setConfirm] = useState(false);
  return (
    <div className="page narrow">
      <Header title="Settings" />
      <section className="card group rise" style={delay(40)}>
        <h2>Connection</h2>
        <Setting label="Control API" note="The local endpoint the CLI, agents and this window talk to."><code>127.0.0.1:{status?.apiPort ?? 7737}</code></Setting>
        <Setting label="Configuration" note="Projects, servers and their commands."><code>%USERPROFILE%\.config\portly</code></Setting>
        <Setting label="Version" note="The daemon that answered last."><span className="num">{status?.version ?? "Unknown"}</span></Setting>
      </section>
      <section className="card group rise" style={delay(90)}>
        <h2>Window</h2>
        <Setting label="Closing the window" note="Portly keeps supervising from the tray. Reopen it from the tray icon.">Hides to the tray</Setting>
        <Setting label="Command palette" note="Jump to any server, start, stop or restart it.">Ctrl K</Setting>
      </section>
      <section className="card group rise" style={delay(140)}>
        <h2>Updates</h2>
        <Setting label="Check for a newer build" note="Opens the release page. A running Portly is never replaced in place.">
          <button className="btn" onClick={onUpdates}><Icon name="update" size={14} />Open releases</button>
        </Setting>
      </section>
      <section className="card group danger rise" style={delay(190)}>
        <Setting label="Quit Portly" note="Stops the daemon and every server it supervises.">
          {confirm ? (
            <span className="confirm">
              <button className="btn" onClick={() => setConfirm(false)}>Keep running</button>
              <button className="btn btn-danger" onClick={onQuit}><Icon name="power" size={14} />Stop everything</button>
            </span>
          ) : (
            <button className="btn btn-danger" onClick={() => setConfirm(true)}><Icon name="power" size={14} />Quit Portly</button>
          )}
        </Setting>
      </section>
    </div>
  );
}

function Setting({ label, note, children }: { label: string; note: string; children: ReactNode }) {
  return (
    <div className="setting">
      <div>
        <strong>{label}</strong>
        <span>{note}</span>
      </div>
      <div className="setting-value">{children}</div>
    </div>
  );
}
