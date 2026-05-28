import { useEffect, useMemo, useRef, useState } from "react";
import type { MutableRefObject } from "react";

import type { EventHistoryHandle } from "@/hooks/useEventHistory";

/**
 * ReplayScrubber — bottom-fixed timeline scrubber that lets the user
 * jump back through cached events. See yass-docs/observability-v2-spec.md
 * §G6.3.
 *
 * NOTE (2026-05-29): visual rewind of the Cesium scene itself (drawing
 * historical satellite positions) is NOT plumbed yet — the Cesium
 * scene continues to render the live tracksRef regardless of
 * scrub position. What this scrubber CAN drive today:
 *
 *   - FileJourneyPanel via the events snapshot at the scrubbed time;
 *   - any future panel that consumes onTimeChange.
 *
 * Full Cesium rewind requires refactoring tracksRef in useSatelliteSse
 * from "latest per fsNode" to a per-fsNode position ring buffer +
 * teaching CesiumScene to honour the scrub time. Tracked in
 * NOTES.md "Deferred obs-v2 follow-ups".
 */
type Props = {
  historyRef: MutableRefObject<EventHistoryHandle>;
  onTimeChange?: (asOfMs: number | null) => void;
};

const PLAYBACK_SPEEDS = [1, 2, 4] as const;
const TICK_INTERVAL_MS = 100;

export function ReplayScrubber({ historyRef, onTimeChange }: Props) {
  const [enabled, setEnabled] = useState(false);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState<(typeof PLAYBACK_SPEEDS)[number]>(1);
  // Local "scrub time" in ms since epoch. null = live.
  const [scrubAt, setScrubAt] = useState<number | null>(null);
  // Bumped to force re-render so we can show the current cached range.
  const [, setRangeTick] = useState(0);
  const rangeTimerRef = useRef<number | null>(null);

  // Re-render every second so the slider's max keeps up with newly
  // captured events.
  useEffect(() => {
    if (!enabled) return;
    rangeTimerRef.current = window.setInterval(() => setRangeTick((n) => n + 1), 1000);
    return () => {
      if (rangeTimerRef.current !== null) window.clearInterval(rangeTimerRef.current);
    };
  }, [enabled]);

  // Playback ticker: advance scrubAt while playing.
  useEffect(() => {
    if (!playing || scrubAt === null) return;
    const handle = window.setInterval(() => {
      setScrubAt((prev) => {
        if (prev === null) return prev;
        const latest = historyRef.current.latestAt;
        if (latest === null) return prev;
        const next = prev + TICK_INTERVAL_MS * speed;
        if (next >= latest) {
          setPlaying(false);
          return latest;
        }
        return next;
      });
    }, TICK_INTERVAL_MS);
    return () => window.clearInterval(handle);
  }, [playing, scrubAt, speed, historyRef]);

  // Propagate scrub changes to consumers.
  useEffect(() => {
    if (!onTimeChange) return;
    onTimeChange(enabled ? scrubAt : null);
  }, [enabled, scrubAt, onTimeChange]);

  const { earliestAt, latestAt } = historyRef.current;
  const range = useMemo(() => {
    if (earliestAt === null || latestAt === null) return null;
    if (latestAt <= earliestAt) return null;
    return { min: earliestAt, max: latestAt };
  }, [earliestAt, latestAt]);

  if (!enabled) {
    return (
      <button
        className="absolute bottom-4 left-1/2 z-10 -translate-x-1/2 rounded-md border border-border bg-background/80 px-3 py-1.5 text-xs font-medium text-foreground/80 shadow-md backdrop-blur-sm hover:bg-background"
        onClick={() => {
          setEnabled(true);
          setScrubAt(historyRef.current.latestAt);
        }}
        title="Replay cached events"
      >
        ⏮ Enable replay
      </button>
    );
  }

  return (
    <div className="absolute bottom-4 left-1/2 z-10 w-[36rem] -translate-x-1/2 rounded-md border border-border bg-background/95 px-3 py-2 shadow-lg backdrop-blur-sm">
      <div className="flex items-center gap-2 text-xs">
        <button
          onClick={() => setPlaying((p) => !p)}
          disabled={range === null || scrubAt === null}
          className="rounded border border-border px-2 py-0.5 hover:bg-muted disabled:opacity-40"
        >
          {playing ? "⏸" : "▶"}
        </button>
        {PLAYBACK_SPEEDS.map((s) => (
          <button
            key={s}
            onClick={() => setSpeed(s)}
            className={`rounded border px-1.5 py-0.5 ${
              speed === s ? "border-emerald-500 text-emerald-300" : "border-border text-foreground/70"
            }`}
          >
            {s}×
          </button>
        ))}
        <button
          onClick={() => {
            setEnabled(false);
            setPlaying(false);
            setScrubAt(null);
          }}
          className="ml-auto rounded border border-border px-2 py-0.5 text-foreground/70 hover:bg-muted"
        >
          Close
        </button>
      </div>
      <div className="mt-2 flex items-center gap-2">
        <span className="font-mono text-[10px] text-muted-foreground">
          {range ? formatHHMMSS(range.min) : "—"}
        </span>
        <input
          type="range"
          min={range?.min ?? 0}
          max={range?.max ?? 1}
          value={scrubAt ?? range?.max ?? 0}
          disabled={range === null}
          onChange={(e) => setScrubAt(Number(e.target.value))}
          className="flex-1 accent-emerald-500"
        />
        <span className="font-mono text-[10px] text-muted-foreground">
          {range ? formatHHMMSS(range.max) : "—"}
        </span>
      </div>
      <div className="mt-1 text-center font-mono text-[10px] text-foreground/80">
        {scrubAt !== null && range
          ? `T = ${formatHHMMSS(scrubAt)} (Δ ${formatOffset(scrubAt - range.max)})`
          : "no events cached"}
      </div>
    </div>
  );
}

function formatHHMMSS(ms: number) {
  const d = new Date(ms);
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}:${pad2(d.getSeconds())}`;
}

function pad2(n: number) {
  return n < 10 ? `0${n}` : `${n}`;
}

function formatOffset(deltaMs: number) {
  if (deltaMs === 0) return "live";
  const s = Math.round(deltaMs / 1000);
  if (s <= -3600) return `${(s / 3600).toFixed(1)}h ago`;
  if (s <= -60) return `${(s / 60).toFixed(1)}m ago`;
  return `${s}s`;
}
