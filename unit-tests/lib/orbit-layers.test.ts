import { describe, it, expect } from "vitest";
import {
  DEFAULT_ORBIT_LAYER_VISIBILITY,
  ORBIT_LAYERS,
  approxGeodeticLatFromCircularOrbit,
  geodeticLatFromOrbitAtElapsedSeconds,
} from "@/lib/orbit-layers";

describe("ORBIT_LAYERS", () => {
  it("defines LEO, MEO, GEO with stable ids", () => {
    const ids = ORBIT_LAYERS.map((l) => l.id);
    expect(ids).toEqual(["LEO", "MEO", "GEO"]);
  });
});

describe("DEFAULT_ORBIT_LAYER_VISIBILITY", () => {
  it("defaults all layers on", () => {
    expect(DEFAULT_ORBIT_LAYER_VISIBILITY).toEqual({ LEO: true, MEO: true, GEO: true });
  });
});

describe("approxGeodeticLatFromCircularOrbit", () => {
  it("matches geodeticLatFromOrbitAtElapsedSeconds at t=0", () => {
    const sat = { altitude: 400, inclination: 98.2, startAngle: 0.7 };
    const approx = approxGeodeticLatFromCircularOrbit(sat);
    const fromElapsed = geodeticLatFromOrbitAtElapsedSeconds(
      sat.altitude,
      sat.inclination,
      sat.startAngle,
      0
    );
    expect(fromElapsed).toBeCloseTo(approx, 8);
  });
});

describe("geodeticLatFromOrbitAtElapsedSeconds", () => {
  it("returns latitude in valid range", () => {
    const lat = geodeticLatFromOrbitAtElapsedSeconds(550, 87.5, 1.1, 120);
    expect(lat).toBeGreaterThanOrEqual(-90);
    expect(lat).toBeLessThanOrEqual(90);
  });
});
