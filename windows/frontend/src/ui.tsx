import { useEffect, useRef, useState, type ReactNode } from "react";
import { HISTORY, stateLabel, type ServerState } from "./lib";

const ICONS: Record<string, ReactNode> = {
  home: <><path d="M2.5 7 8 2.5 13.5 7v6a.5.5 0 0 1-.5.5h-3v-4H6v4H3a.5.5 0 0 1-.5-.5Z" /></>,
  clock: <><circle cx="8" cy="8" r="6" /><path d="M8 4.5V8l2.5 1.5" /></>,
  chart: <path d="M2 13V8M6 13V3M10 13V6M14 13V10" />,
  gear: <><circle cx="8" cy="8" r="2" /><path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.4 3.4l1.4 1.4M11.2 11.2l1.4 1.4M3.4 12.6l1.4-1.4M11.2 4.8l1.4-1.4" /></>,
  play: <path d="M5 3.2v9.6L12.5 8Z" fill="currentColor" />,
  stop: <rect x="4" y="4" width="8" height="8" rx="1.5" fill="currentColor" />,
  restart: <><path d="M13 8a5 5 0 1 1-1.5-3.6" /><path d="M12 1.8v3h-3" /></>,
  search: <><circle cx="7" cy="7" r="4.5" /><path d="m10.5 10.5 3 3" /></>,
  copy: <><rect x="5.5" y="5.5" width="8" height="8" rx="1.5" /><path d="M10.5 5.5V3.8a1.3 1.3 0 0 0-1.3-1.3H3.8a1.3 1.3 0 0 0-1.3 1.3v5.4a1.3 1.3 0 0 0 1.3 1.3h1.7" /></>,
  pause: <path d="M5.5 3.5v9M10.5 3.5v9" />,
  follow: <path d="M8 3v9.5M4 8.5l4 4 4-4" />,
  external: <><path d="M9.5 2.5h4v4" /><path d="m13.5 2.5-6 6" /><path d="M12 9.5v3a1 1 0 0 1-1 1H3.5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h3" /></>,
  folder: <path d="M2 4.5a1 1 0 0 1 1-1h3.2l1.5 1.5H13a1 1 0 0 1 1 1V12a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1Z" />,
  back: <path d="M10 3 5 8l5 5" />,
  chevron: <path d="m6 3.5 4.5 4.5L6 12.5" />,
  enter: <><path d="M13 3v5a2 2 0 0 1-2 2H3" /><path d="m5.5 7.5-2.5 2.5 2.5 2.5" /></>,
  bolt: <path d="M9 1.5 3.5 9H8l-1 5.5L12.5 7H8Z" />,
  update: <><path d="M8 2v8.5M4.5 7 8 10.5 11.5 7" /><path d="M2.5 13.5h11" /></>,
  power: <><path d="M8 1.8v5.5" /><path d="M4.4 4a5.3 5.3 0 1 0 7.2 0" /></>,
};

export function Icon({ name, size = 16 }: { name: keyof typeof ICONS | string; size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      {ICONS[name] ?? <circle cx="8" cy="8" r="5" />}
    </svg>
  );
}

/** The app icon: a three-unit rack, the middle one live. */
export function Logo({ size = 16 }: { size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 1024 1024" aria-hidden>
      <rect x="64" y="64" width="896" height="896" rx="200" fill="#16161a" />
      <g strokeLinecap="round" strokeWidth="136" fill="none">
        <path d="M300 330h424M300 694h424" stroke="#8fd3a8" />
        <path d="M300 512h424" stroke="#a99cf2" />
      </g>
      <g fill="#16161a">
        <circle cx="312" cy="330" r="30" />
        <circle cx="312" cy="512" r="30" />
        <circle cx="312" cy="694" r="30" />
      </g>
    </svg>
  );
}

export function Dot({ state }: { state: ServerState }) {
  return <span className={`dot dot-${state}`} role="img" aria-label={stateLabel(state)} />;
}

export function StatePill({ state }: { state: ServerState }) {
  return (
    <span className={`state state-${state}`}>
      <Dot state={state} />
      {stateLabel(state)}
    </span>
  );
}

/**
 * A rolling line drawn from the samples. New samples enter on the right, so the
 * x step is fixed by the history length instead of by how many samples exist yet.
 */
export function Sparkline({ values, color = "var(--live)", height = 40, max, className = "" }: { values: number[]; color?: string; height?: number; max?: number; className?: string }) {
  const width = 300;
  // Until the window holds a full history, stretch what exists so the line never looks cut off.
  const step = width / (Math.min(HISTORY, Math.max(values.length, 2)) - 1);
  const top = Math.max(max ?? 0, ...values, 1) * 1.12;
  const points = values.map((value, index) => [width - (values.length - 1 - index) * step, height - 2 - (value / top) * (height - 4)] as const);
  const line = points.map(([x, y], index) => `${index ? "L" : "M"}${x.toFixed(1)},${y.toFixed(1)}`).join("");
  const area = points.length > 1 ? `${line}L${width},${height}L${points[0][0].toFixed(1)},${height}Z` : "";
  return (
    <svg className={`spark ${className}`} viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" height={height} aria-hidden>
      {points.length > 1 && (
        <>
          <path d={area} fill={color} opacity="0.12" />
          <path d={line} fill="none" stroke={color} strokeWidth="1.6" vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
          <circle className="spark-head" cx={points[points.length - 1][0] - 1.5} cy={points[points.length - 1][1]} r="2.5" fill={color} vectorEffect="non-scaling-stroke" />
        </>
      )}
    </svg>
  );
}

/** Eases a number toward its new value instead of jumping. */
export function useTween(target: number, duration = 600) {
  const [value, setValue] = useState(target);
  const from = useRef(target);
  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      from.current = target;
      setValue(target);
      return;
    }
    const origin = from.current;
    const start = performance.now();
    let frame = 0;
    const tick = (now: number) => {
      const t = Math.min(1, (now - start) / duration);
      const eased = 1 - Math.pow(1 - t, 3);
      const next = origin + (target - origin) * eased;
      from.current = next;
      setValue(next);
      if (t < 1) frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [target, duration]);
  return value;
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="kbd">{children}</kbd>;
}
