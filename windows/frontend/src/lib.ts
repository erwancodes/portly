export type ServerState = "starting" | "running" | "stopped" | "failed" | "restarting";

export type ServerStatus = {
  id: string;
  name: string;
  projectID: string;
  projectName: string;
  command: string;
  port: number | null;
  directory: string;
  state: ServerState;
  pid: number | null;
  startedAt: string | null;
  restartCount: number;
  lastExitCode: number | null;
  lastError: string | null;
  healthy: boolean;
  url: string | null;
  cpuPercent?: number;
  memoryBytes?: number;
  residentMemoryBytes?: number;
  processCount?: number;
  temporary?: boolean;
  finishedAt?: string | null;
};

export type ProjectStatus = {
  id: string;
  name: string;
  icon: string;
  color: string;
  root: string;
  servers: ServerStatus[];
};

export type PortlyStatus = {
  version: string;
  apiPort: number;
  projects: ProjectStatus[];
  temporaryServers: ServerStatus[];
};

export type LogsResponse = { server: string; lines: string[] };
export type Verb = "start" | "stop" | "restart";

type PortlyBridge = {
  GetStatus: () => Promise<string>;
  GetLogs: (server: string, tail: number) => Promise<string>;
  WriteTerminal: (server: string, data: string) => Promise<string>;
  ResizeTerminal: (server: string, width: number, height: number) => Promise<string>;
  Action: (verb: string, target: string) => Promise<string>;
  OpenReleases: () => void;
  Quit: () => Promise<void>;
};

type WailsRuntime = {
  WindowMinimise: () => void;
  WindowToggleMaximise: () => void;
  WindowHide: () => void;
  BrowserOpenURL: (url: string) => void;
};

declare global {
  interface Window {
    go?: { main?: { App?: PortlyBridge } };
    runtime?: WailsRuntime;
  }
}

async function unwrap(response: Response): Promise<string> {
  const envelope = (await response.json()) as { ok: boolean; data: unknown; error?: string | null };
  if (!envelope.ok) throw new Error(envelope.error ?? "Portly request failed");
  return JSON.stringify(envelope.data);
}

/**
 * Outside the Wails shell (vite dev), read the real daemon through the dev proxy.
 * Everything that changes state is a no-op there, so a preview never touches a server.
 */
const devBridge: PortlyBridge = {
  GetStatus: async () => unwrap(await fetch("/__portly/status")),
  GetLogs: async (server, tail) => unwrap(await fetch(`/__portly/logs?server=${encodeURIComponent(server)}&tail=${tail}`)),
  WriteTerminal: async () => "{}",
  ResizeTerminal: async () => "{}",
  Action: async () => "{}",
  OpenReleases: () => window.open("https://github.com/", "_blank"),
  Quit: async () => undefined,
};

export const inShell = Boolean(window.go?.main?.App);

export function bridge(): PortlyBridge {
  const app = window.go?.main?.App;
  if (app) return app;
  if (import.meta.env.DEV) return devBridge;
  throw new Error("Portly is not running inside the Windows shell.");
}

export function openURL(url: string) {
  if (window.runtime?.BrowserOpenURL) window.runtime.BrowserOpenURL(url);
  else window.open(url, "_blank");
}

export const parse = <T,>(raw: string): T => JSON.parse(raw) as T;

export const serverKey = (server: ServerStatus) => `${server.projectName}/${server.name}`;

export function allServers(status: PortlyStatus | null): ServerStatus[] {
  if (!status) return [];
  return [...status.projects.flatMap((project) => project.servers), ...status.temporaryServers];
}

/**
 * The daemon reports no metrics at all when a process snapshot fails. When
 * every live server comes back empty, carry each one's previous numbers so the
 * charts and totals do not drop to zero for a tick.
 */
export function keepMetrics(previous: PortlyStatus | null, next: PortlyStatus): PortlyStatus {
  const live = allServers(next).filter((server) => isLive(server.state));
  if (!previous || live.length === 0 || live.some((server) => server.memoryBytes || server.cpuPercent)) return next;
  const before = new Map(allServers(previous).map((server) => [serverKey(server), server]));
  const patch = (server: ServerStatus): ServerStatus => {
    const old = before.get(serverKey(server));
    return isLive(server.state) && old ? { ...server, memoryBytes: old.memoryBytes, cpuPercent: old.cpuPercent } : server;
  };
  return {
    ...next,
    projects: next.projects.map((project) => ({ ...project, servers: project.servers.map(patch) })),
    temporaryServers: next.temporaryServers.map(patch),
  };
}

export const isLive = (state: ServerState) => state === "running" || state === "starting" || state === "restarting";

export function stateLabel(state: ServerState): string {
  return { running: "Running", starting: "Starting", restarting: "Restarting", failed: "Failed", stopped: "Stopped" }[state];
}

export function formatBytes(value?: number): string {
  if (!value) return "0 MB";
  if (value >= 1024 ** 3) return `${(value / 1024 ** 3).toFixed(1)} GB`;
  return `${Math.round(value / 1024 ** 2)} MB`;
}

export function splitBytes(value?: number): [string, string] {
  const [number, unit] = formatBytes(value).split(" ");
  return [number, unit];
}

export function since(iso: string | null | undefined, until?: string | null): string {
  if (!iso) return "";
  const start = Date.parse(iso);
  const end = until ? Date.parse(until) : Date.now();
  if (Number.isNaN(start)) return "";
  const seconds = Math.max(0, Math.floor((end - start) / 1000));
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ${minutes % 60}m`;
  return `${Math.floor(hours / 24)}d ${hours % 24}h`;
}

export function projectColor(color?: string) {
  return color && /^#[0-9a-f]{3,8}$/i.test(color) ? color : "#8fd3a8";
}

/** Rolling samples kept in the window, not on disk: about three minutes at one sample every two seconds. */
export const HISTORY = 90;
export type Series = { memory: number[]; cpu: number[] };
export type History = Record<string, Series>;
export const TOTAL = "__total__";

export function record(history: History, status: PortlyStatus): History {
  const next: History = {};
  const push = (key: string, memory: number, cpu: number) => {
    const previous = history[key] ?? { memory: [], cpu: [] };
    next[key] = { memory: [...previous.memory, memory].slice(-HISTORY), cpu: [...previous.cpu, cpu].slice(-HISTORY) };
  };
  let memory = 0;
  let cpu = 0;
  for (const server of allServers(status)) {
    const m = isLive(server.state) ? server.memoryBytes ?? 0 : 0;
    const c = isLive(server.state) ? server.cpuPercent ?? 0 : 0;
    memory += m;
    cpu += c;
    push(serverKey(server), m, c);
  }
  push(TOTAL, memory, cpu);
  return next;
}
