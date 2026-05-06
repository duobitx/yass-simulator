import { describe, it, expect } from "vitest";
import {
  getECIPosition,
  getOrbitColor,
  getOrbitType,
  getSatellitePosition,
  parseTLE,
  predictPasses,
  sampleTLEs,
} from "@/lib/tle-service";

describe("getOrbitType", () => {
  it("classifies altitude bands", () => {
    expect(getOrbitType(500)).toBe("LEO");
    expect(getOrbitType(10000)).toBe("MEO");
    expect(getOrbitType(36000)).toBe("GEO");
  });
});

describe("getOrbitColor", () => {
  it("returns hex colors per regime", () => {
    expect(getOrbitColor("LEO")).toMatch(/^#/);
    expect(getOrbitColor("MEO")).toMatch(/^#/);
    expect(getOrbitColor("GEO")).toMatch(/^#/);
  });
});

describe("parseTLE", () => {
  it("parses sample TLEs", () => {
    for (const tle of sampleTLEs) {
      const rec = parseTLE(tle);
      expect(rec).not.toBeNull();
    }
  });
});

describe("getSatellitePosition", () => {
  it("returns lat/lon/alt/velocity for a valid satrec", () => {
    const rec = parseTLE(sampleTLEs[0]!);
    expect(rec).not.toBeNull();
    const pos = getSatellitePosition(rec!, new Date("2024-06-01T12:00:00Z"));
    expect(pos).not.toBeNull();
    expect(pos!.latitude).toBeGreaterThanOrEqual(-90);
    expect(pos!.latitude).toBeLessThanOrEqual(90);
    expect(pos!.longitude).toBeGreaterThanOrEqual(-180);
    expect(pos!.longitude).toBeLessThanOrEqual(180);
    expect(pos!.altitude).toBeGreaterThan(0);
    expect(pos!.velocity).toBeGreaterThan(0);
  });
});

describe("getECIPosition", () => {
  it("returns scaled ECI coordinates", () => {
    const rec = parseTLE(sampleTLEs[0]!);
    expect(rec).not.toBeNull();
    const eci = getECIPosition(rec!, new Date("2024-06-01T12:00:00Z"));
    expect(eci).not.toBeNull();
    expect(Math.abs(eci!.x)).toBeGreaterThan(0);
  });
});

describe("predictPasses", () => {
  it("returns an array (possibly empty) without throwing", () => {
    const rec = parseTLE(sampleTLEs[0]!);
    expect(rec).not.toBeNull();
    const passes = predictPasses(
      rec!,
      { lat: 50.85, lng: 4.35 },
      new Date("2024-06-01T00:00:00Z"),
      6
    );
    expect(Array.isArray(passes)).toBe(true);
    expect(passes.length).toBeLessThanOrEqual(10);
  });
});
