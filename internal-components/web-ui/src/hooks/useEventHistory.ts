import { useEffect, useRef } from "react";
import type { MutableRefObject } from "react";

import type { SseAgentFileEvent, SseHardwareEvent } from "@/lib/sse-types";

/**
 * useEventHistory — captures every file-lifecycle and hardware-event
 * received over SSE into an unbounded-for-the-tab-lifetime in-memory
 * ring buffer, with their wall timestamps.
 *
 * The ref is read by ReplayScrubber to determine the available time
 * range and by the consuming widgets (file journey, fault overlay) when
 * the user has scrubbed back in time.
 *
 * Per the v2 spec §5.5: "client caches events into IndexedDB for the
 * duration the tab is open". For MVP we use plain memory — IndexedDB
 * spill is a follow-up. See yass-docs/observability-v2-spec.md §G6.3.
 *
 * Inputs are the live refs from useSatelliteSse; we poll them every
 * tick to detect appends (the refs are mutated in place — React does
 * not signal). The poll is cheap because we keep a per-source cursor.
 */
type SnapshotInputs = {
  eventsRef: MutableRefObject<Record<string, SseAgentFileEvent[]>>;
  faultsRef: MutableRefObject<Record<string, Set<string>>>;
};

export type EventHistoryEntry =
  | { kind: "file"; at: number; payload: SseAgentFileEvent }
  | {
      kind: "fault";
      at: number;
      fsNode: string;
      activeTypes: string[]; // snapshot of the fsNode's fault set
    };

export type EventHistoryHandle = {
  current: EventHistoryEntry[];
  earliestAt: number | null;
  latestAt: number | null;
};

const POLL_INTERVAL_MS = 500;

export function useEventHistory({ eventsRef, faultsRef }: SnapshotInputs): MutableRefObject<EventHistoryHandle> {
  // Per-source cursor used to detect appends in the eventsRef without
  // re-walking the whole buffer every tick.
  const seenRef = useRef<Map<string, Set<string>>>(new Map());
  const lastFaultRef = useRef<Map<string, string>>(new Map());
  const historyRef = useRef<EventHistoryHandle>({
    current: [],
    earliestAt: null,
    latestAt: null,
  });

  useEffect(() => {
    const tick = () => {
      const now = Date.now();
      let mutated = false;

      // 1. File events: append anything we haven't seen.
      for (const [fsNode, evs] of Object.entries(eventsRef.current)) {
        let seen = seenRef.current.get(fsNode);
        if (!seen) {
          seen = new Set();
          seenRef.current.set(fsNode, seen);
        }
        for (const e of evs) {
          const key = `${e.timestamp}|${e.fileName}|${e.action}`;
          if (seen.has(key)) continue;
          seen.add(key);
          const at = Date.parse(e.timestamp) || now;
          historyRef.current.current.push({ kind: "file", at, payload: e });
          mutated = true;
        }
      }

      // 2. Fault snapshots: append whenever the fsNode's set changes.
      for (const [fsNode, set] of Object.entries(faultsRef.current)) {
        const sig = [...set].sort().join(",");
        if (lastFaultRef.current.get(fsNode) === sig) continue;
        lastFaultRef.current.set(fsNode, sig);
        historyRef.current.current.push({
          kind: "fault",
          at: now,
          fsNode,
          activeTypes: [...set].sort(),
        });
        mutated = true;
      }

      if (mutated) {
        historyRef.current.current.sort((a, b) => a.at - b.at);
        historyRef.current.earliestAt = historyRef.current.current[0]?.at ?? null;
        historyRef.current.latestAt =
          historyRef.current.current[historyRef.current.current.length - 1]?.at ?? null;
      }
    };

    tick();
    const handle = window.setInterval(tick, POLL_INTERVAL_MS);
    return () => window.clearInterval(handle);
  }, [eventsRef, faultsRef]);

  return historyRef;
}

/**
 * filterHistoryAt returns every history entry with at ≤ asOfMs. For
 * fault entries it picks the LATEST snapshot per fsNode at or before
 * the cutoff. For file entries it returns all PUTs/RECEIVEDs in order.
 */
export function filterHistoryAt(history: EventHistoryEntry[], asOfMs: number) {
  const files: SseAgentFileEvent[] = [];
  const faultsByNode = new Map<string, string[]>();
  for (const entry of history) {
    if (entry.at > asOfMs) break; // history is sorted by `at`
    if (entry.kind === "file") {
      files.push(entry.payload);
    } else {
      faultsByNode.set(entry.fsNode, entry.activeTypes);
    }
  }
  return { files, faultsByNode };
}
