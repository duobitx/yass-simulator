import { useEffect, useRef, useState } from "react";
import type { SsePositionEvent } from "@/lib/sse-types";

const FLUSH_MS = 150;

function parseSsePayloadLine(line: string): unknown {
  const t = line.replace(/\r$/, "").trim();
  if (!t) return null;
  let payload = t;
  if (payload.startsWith("data:")) payload = payload.slice(5).trim();
  if (payload === "[DONE]") return null;
  try {
    return JSON.parse(payload);
  } catch {
    return null;
  }
}

export type SseConnectionStatus = "idle" | "connecting" | "live" | "error" | "closed";

export function useSatelliteSse(url: string, enabled: boolean) {
  const tracksRef = useRef<Record<string, SsePositionEvent>>({});
  const [tracks, setTracks] = useState<Record<string, SsePositionEvent>>({});
  const [sourceSignature, setSourceSignature] = useState("");
  const [status, setStatus] = useState<SseConnectionStatus>("idle");
  const sigRef = useRef("");
  const flushTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (!enabled) {
      if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
      flushTimerRef.current = null;
      tracksRef.current = {};
      sigRef.current = "";
      setTracks({});
      setSourceSignature("");
      setStatus("idle");
      return;
    }

    const ac = new AbortController();
    let cancelled = false;

    const flush = () => {
      flushTimerRef.current = null;
      setTracks({ ...tracksRef.current });
    };

    const scheduleFlush = () => {
      if (flushTimerRef.current != null) return;
      flushTimerRef.current = setTimeout(flush, FLUSH_MS);
    };

    const run = async () => {
      tracksRef.current = {};
      sigRef.current = "";
      setSourceSignature("");
      setTracks({});
      setStatus("connecting");
      try {
        const res = await fetch(url, {
          headers: { Accept: "text/event-stream" },
          signal: ac.signal,
        });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const reader = res.body?.getReader();
        if (!reader) throw new Error("No response body");
        const dec = new TextDecoder();
        let buf = "";
        setStatus("live");
        while (!cancelled) {
          const { done, value } = await reader.read();
          if (done) break;
          buf += dec.decode(value, { stream: true });
          const lines = buf.split("\n");
          buf = lines.pop() ?? "";
          for (const raw of lines) {
            const row = parseSsePayloadLine(raw);
            if (!row || typeof row !== "object") continue;
            const ev = row as Partial<SsePositionEvent>;
            if (ev.eventType !== "PositionEvent" || typeof ev.source !== "string") continue;
            const full = ev as SsePositionEvent;
            tracksRef.current = { ...tracksRef.current, [full.source]: full };
            const sig = Object.keys(tracksRef.current).sort().join(",");
            if (sig !== sigRef.current) {
              sigRef.current = sig;
              setSourceSignature(sig);
            }
            scheduleFlush();
          }
        }
        if (!cancelled) setStatus("closed");
      } catch {
        if (!ac.signal.aborted && !cancelled) setStatus("error");
      } finally {
        if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
        flushTimerRef.current = null;
      }
    };

    run();

    return () => {
      cancelled = true;
      ac.abort();
      if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
      flushTimerRef.current = null;
    };
  }, [url, enabled]);

  return { tracks, tracksRef, sourceSignature, status };
}
