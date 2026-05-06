import { describe, it, expect } from "vitest";
import type { SsePositionEvent } from "@/lib/sse-types";
import {
  formatTrackLabel,
  groundFromSseTrack,
  listsFromSseTracks,
  satelliteFromSseTrack,
} from "@/lib/sse-track-utils";

describe("formatTrackLabel", () => {
  it("strips estrack prefix and title-cases segments", () => {
    expect(formatTrackLabel("estrack-leo-sat-01")).toBe("Leo Sat 01");
  });

  it("handles ids without prefix", () => {
    expect(formatTrackLabel("ground-station-x")).toBe("Ground Station X");
  });
});

describe("satelliteFromSseTrack", () => {
  it("classifies LEO from altitude and sets rounded altitude", () => {
    const ev: SsePositionEvent = {
      source: "s",
      timestamp: "t",
      eventType: "e",
      X: 0,
      Y: 0,
      Z: 0,
      Lat: 10,
      Lng: 20,
      Alt: 500,
    };
    const sat = satelliteFromSseTrack("track-a", ev);
    expect(sat.id).toBe("track-a");
    expect(sat.orbitType).toBe("LEO");
    expect(sat.altitude).toBe(500);
    expect(sat.inclination).toBe(53);
  });

  it("classifies GEO and uses near-zero inclination", () => {
    const ev: SsePositionEvent = {
      source: "s",
      timestamp: "t",
      eventType: "e",
      X: 0,
      Y: 0,
      Z: 0,
      Lat: 0,
      Lng: 0,
      Alt: 35786,
    };
    const sat = satelliteFromSseTrack("geo-x", ev);
    expect(sat.orbitType).toBe("GEO");
    expect(sat.inclination).toBe(0.1);
  });
});

describe("groundFromSseTrack", () => {
  it("maps Lat/Lng from event", () => {
    const ev: SsePositionEvent = {
      source: "s",
      timestamp: "t",
      eventType: "e",
      X: 0,
      Y: 0,
      Z: 0,
      Lat: 50.1,
      Lng: 4.2,
      Alt: 0,
    };
    const gs = groundFromSseTrack("gs-1", ev);
    expect(gs.lat).toBe(50.1);
    expect(gs.lon).toBe(4.2);
  });
});

describe("listsFromSseTracks", () => {
  it("returns empty when sourceSignature is empty", () => {
    expect(listsFromSseTracks({}, "")).toEqual({ satellites: [], groundStations: [] });
  });

  it("splits ids, skips missing tracks, splits ground vs orbit", () => {
    const tracks: Record<string, SsePositionEvent> = {
      sat1: {
        source: "s",
        timestamp: "t",
        eventType: "e",
        X: 0,
        Y: 0,
        Z: 0,
        Lat: 0,
        Lng: 0,
        Alt: 400,
      },
      g1: {
        source: "s",
        timestamp: "t",
        eventType: "e",
        X: 0,
        Y: 0,
        Z: 0,
        Lat: 51,
        Lng: 5,
        Alt: 0,
      },
    };
    const out = listsFromSseTracks(tracks, "sat1,missing,g1");
    expect(out.satellites).toHaveLength(1);
    expect(out.satellites[0].id).toBe("sat1");
    expect(out.groundStations).toHaveLength(1);
    expect(out.groundStations[0].id).toBe("g1");
  });
});
