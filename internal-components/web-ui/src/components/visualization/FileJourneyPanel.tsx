import { useEffect, useState } from "react";
import type { MutableRefObject } from "react";

import type { SseAgentFileEvent } from "@/lib/sse-types";

/**
 * FileJourneyPanel — side widget listing files currently in flight or
 * recently delivered. Each entry shows:
 *   - source satellite that PUT the file (highlighted by md5);
 *   - every fsNode that has RECEIVED a copy so far, with the delivery
 *     time vs the original PUT;
 *   - turns green when the file has been received by ≥1 ground station.
 *
 * Data source: eventsRef from useSatelliteSse, which mirrors the
 * `AgentFileEvent` stream (PUT / RECEIVED / DELETE) per fsNode. We pull
 * snapshots every 1s instead of subscribing to a stream because the ref
 * is mutated in place (no React signal).
 *
 * See yass-docs/observability-v2-spec.md §G6.2.
 */
type Props = {
  eventsRef: MutableRefObject<Record<string, SseAgentFileEvent[]>>;
  // groundStationNames so we can colour PUTs that have reached at least
  // one GS green. Optional — if unset every delivery is grey-on-grey.
  groundStationNames?: Set<string>;
};

type JourneyEntry = {
  md5: string;
  fileName: string;
  source: string;
  sourceTs: number;
  size?: number;
  deliveries: Delivery[];
};

type Delivery = {
  target: string;
  ts: number;
  deltaSeconds: number;
};

const REFRESH_INTERVAL_MS = 1000;
const MAX_VISIBLE_ENTRIES = 8;

export function FileJourneyPanel({ eventsRef, groundStationNames }: Props) {
  const [entries, setEntries] = useState<JourneyEntry[]>([]);

  useEffect(() => {
    const tick = () => setEntries(buildEntries(eventsRef.current));
    tick();
    const handle = window.setInterval(tick, REFRESH_INTERVAL_MS);
    return () => window.clearInterval(handle);
  }, [eventsRef]);

  if (entries.length === 0) return null;

  return (
    <div className="absolute bottom-4 right-4 z-10 w-80 rounded-md border border-border bg-background/95 p-3 shadow-lg backdrop-blur-sm">
      <div className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        Files in flight
      </div>
      <ul className="max-h-96 space-y-2 overflow-y-auto text-sm">
        {entries.slice(0, MAX_VISIBLE_ENTRIES).map((e) => (
          <FileRow key={e.md5} entry={e} groundStationNames={groundStationNames} />
        ))}
      </ul>
      {entries.length > MAX_VISIBLE_ENTRIES && (
        <div className="mt-2 text-[10px] text-muted-foreground">
          +{entries.length - MAX_VISIBLE_ENTRIES} more not shown
        </div>
      )}
    </div>
  );
}

function FileRow({
  entry,
  groundStationNames,
}: {
  entry: JourneyEntry;
  groundStationNames?: Set<string>;
}) {
  const reachedGS =
    groundStationNames &&
    entry.deliveries.some((d) => groundStationNames.has(d.target));
  return (
    <li className="rounded-sm border border-border/50 px-2 py-1">
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-mono text-xs">{entry.fileName || entry.md5.slice(0, 12)}</span>
        <span
          className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-medium ${
            reachedGS
              ? "bg-emerald-500/20 text-emerald-300"
              : "bg-muted text-muted-foreground"
          }`}
        >
          {entry.deliveries.length === 0
            ? "in flight"
            : `${entry.deliveries.length} hop${entry.deliveries.length === 1 ? "" : "s"}`}
        </span>
      </div>
      <div className="mt-1 text-[10px] text-muted-foreground">
        <span className="font-medium text-foreground/80">{entry.source}</span>
        {entry.deliveries.length === 0
          ? " → waiting"
          : entry.deliveries.map((d) => (
              <span key={`${entry.md5}-${d.target}`}>
                {" → "}
                <span className={groundStationNames?.has(d.target) ? "text-emerald-400" : ""}>
                  {d.target}
                </span>
                <span className="ml-1 opacity-60">({formatDelta(d.deltaSeconds)})</span>
              </span>
            ))}
      </div>
    </li>
  );
}

function buildEntries(perNode: Record<string, SseAgentFileEvent[]>): JourneyEntry[] {
  // Walk every fsNode's recent events and group by md5. The eventsRef
  // is bounded per node (EVENTS_HISTORY_PER_FSNODE) so this is cheap.
  const byMd5 = new Map<string, JourneyEntry>();
  for (const events of Object.values(perNode)) {
    for (const e of events) {
      if (!e.md5) continue;
      let entry = byMd5.get(e.md5);
      if (!entry) {
        entry = {
          md5: e.md5,
          fileName: e.fileName,
          source: "",
          sourceTs: 0,
          size: e.contentSizeBytes,
          deliveries: [],
        };
        byMd5.set(e.md5, entry);
      }
      const ts = Date.parse(e.timestamp);
      if (e.action === "PUT") {
        entry.source = e.source;
        entry.sourceTs = ts;
        if (e.fileName) entry.fileName = e.fileName;
        if (e.contentSizeBytes !== undefined) entry.size = e.contentSizeBytes;
      } else if (e.action === "RECEIVED") {
        if (entry.deliveries.some((d) => d.target === e.source)) continue;
        entry.deliveries.push({
          target: e.source,
          ts,
          deltaSeconds: entry.sourceTs > 0 ? (ts - entry.sourceTs) / 1000 : 0,
        });
      }
    }
  }
  const out = Array.from(byMd5.values()).filter((e) => e.source !== "");
  // Sort by most-recent activity first.
  out.sort((a, b) => {
    const aLast = Math.max(a.sourceTs, ...a.deliveries.map((d) => d.ts));
    const bLast = Math.max(b.sourceTs, ...b.deliveries.map((d) => d.ts));
    return bLast - aLast;
  });
  for (const e of out) e.deliveries.sort((a, b) => a.ts - b.ts);
  return out;
}

function formatDelta(s: number): string {
  if (s < 0) return "?";
  if (s < 1) return `${Math.round(s * 1000)}ms`;
  if (s < 60) return `${s.toFixed(1)}s`;
  return `${(s / 60).toFixed(1)}m`;
}
