import { describe, it, expect } from "vitest";
import {
  defaultExperimentConfig,
  experimentConfigs,
  getExperimentConfig,
} from "@/lib/experiment-configs";

describe("getExperimentConfig", () => {
  it("returns default when experimentId is missing", () => {
    expect(getExperimentConfig()).toBe(defaultExperimentConfig);
    expect(getExperimentConfig(undefined)).toBe(defaultExperimentConfig);
  });

  it("returns a known experiment by id", () => {
    const cfg = getExperimentConfig("EXP-001");
    expect(cfg.id).toBe("EXP-001");
    expect(cfg.name).toBe("Satellite Network Live");
    expect(cfg.satellites.length).toBeGreaterThan(0);
    expect(cfg.groundStations.length).toBeGreaterThan(0);
  });

  it("falls back to default for unknown ids", () => {
    expect(getExperimentConfig("no-such-exp")).toBe(defaultExperimentConfig);
  });
});

describe("experimentConfigs", () => {
  it("contains EXP-DEMO with full demo set", () => {
    const demo = experimentConfigs["EXP-DEMO"];
    expect(demo.satellites.length).toBeGreaterThanOrEqual(8);
    expect(demo.groundStations.length).toBeGreaterThanOrEqual(6);
  });
});
