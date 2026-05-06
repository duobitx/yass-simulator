import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useSatelliteSse } from "@/hooks/useSatelliteSse";

function positionEvent(source: string, alt: number): Record<string, unknown> {
  return {
    eventType: "PositionEvent",
    source,
    timestamp: "2024-01-01T00:00:00Z",
    X: 0,
    Y: 0,
    Z: 0,
    Lat: 0,
    Lng: 0,
    Alt: alt,
  };
}

function sseResponse(lines: string[]) {
  const encoder = new TextEncoder();
  const body = new ReadableStream({
    start(controller) {
      for (const line of lines) {
        controller.enqueue(encoder.encode(`${line}\n`));
      }
      controller.close();
    },
  });
  return new Response(body, { status: 200, statusText: "OK" });
}

describe("useSatelliteSse", () => {
  beforeEach(() => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => sseResponse([`data: ${JSON.stringify(positionEvent("sat-a", 400))}`]))
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("stays idle when disabled and does not fetch", () => {
    const { result } = renderHook(() => useSatelliteSse("http://example/sse", false));
    expect(result.current.status).toBe("idle");
    expect(Object.keys(result.current.tracks)).toHaveLength(0);
    expect(fetch).not.toHaveBeenCalled();
  });

  it("loads PositionEvent rows into ref, updates signature, and ends closed after stream completes", async () => {
    const { result } = renderHook(() => useSatelliteSse("http://example/sse", true));

    // Ref is updated synchronously per line; batched `tracks` state may not flush if the
    // stream ends before the debounce timer fires (see hook implementation).
    await waitFor(() => {
      expect(result.current.tracksRef.current["sat-a"]?.source).toBe("sat-a");
    });
    await waitFor(() => {
      expect(result.current.sourceSignature).toBe("sat-a");
    });
    await waitFor(() => {
      expect(result.current.status).toBe("closed");
    });
    expect(fetch).toHaveBeenCalledWith("http://example/sse", {
      headers: { Accept: "text/event-stream" },
      signal: expect.any(AbortSignal),
    });
  });

  it("sets status error when HTTP fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 503, statusText: "Service Unavailable" }))
    );
    const { result } = renderHook(() => useSatelliteSse("http://example/sse", true));
    await waitFor(() => {
      expect(result.current.status).toBe("error");
    });
  });

  it("ignores non-JSON, wrong eventType, and malformed rows", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        sseResponse([
          `data: not-json`,
          `data: ${JSON.stringify({ eventType: "Other", source: "x" })}`,
          `data: ${JSON.stringify({ ...positionEvent("keep", 300), eventType: "PositionEvent" })}`,
        ])
      )
    );
    const { result } = renderHook(() => useSatelliteSse("http://example/sse", true));
    await waitFor(() => {
      expect(result.current.tracksRef.current["keep"]?.Alt).toBe(300);
    });
    expect(result.current.tracks["x"]).toBeUndefined();
  });
});
