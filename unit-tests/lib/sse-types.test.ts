import { describe, it, expect } from "vitest";
import { isGroundStationEvent, sseEventsUrl } from "@/lib/sse-types";

describe("isGroundStationEvent", () => {
  it("treats zero altitude as ground segment", () => {
    expect(
      isGroundStationEvent({
        source: "s",
        timestamp: "t",
        eventType: "e",
        X: 0,
        Y: 0,
        Z: 0,
        Lat: 1,
        Lng: 2,
        Alt: 0,
      })
    ).toBe(true);
  });

  it("treats non-zero altitude as in orbit", () => {
    expect(
      isGroundStationEvent({
        source: "s",
        timestamp: "t",
        eventType: "e",
        X: 0,
        Y: 0,
        Z: 0,
        Lat: 1,
        Lng: 2,
        Alt: 400,
      })
    ).toBe(false);
  });
});

describe("sseEventsUrl", () => {
  it("returns a non-empty string", () => {
    expect(typeof sseEventsUrl()).toBe("string");
    expect(sseEventsUrl().length).toBeGreaterThan(0);
  });
});
