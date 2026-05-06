import type { SatelliteData } from "@/lib/experiment-configs";

/** Three orbital regimes (matches `SatelliteData.orbitType`). */
export const ORBIT_LAYERS = [
  {
    id: "LEO" as const,
    label: "LEO",
    rangeHint: "Low Earth Orbit (~200–2,000 km)",
    color: "#4ade80",
  },
  {
    id: "MEO" as const,
    label: "MEO",
    rangeHint: "Medium Earth Orbit (~2,000–35,786 km)",
    color: "#facc15",
  },
  {
    id: "GEO" as const,
    label: "GEO",
    rangeHint: "Geosynchronous / GEO (~35,786 km)",
    color: "#f97316",
  },
] as const;

export type OrbitLayerId = (typeof ORBIT_LAYERS)[number]["id"];

export const DEFAULT_ORBIT_LAYER_VISIBILITY: Record<OrbitLayerId, boolean> = {
  LEO: true,
  MEO: true,
  GEO: true,
};

/** Snapshot latitude on circular orbit model (same geometry as Cesium orbit path), angle = startAngle at t = 0. */
export function approxGeodeticLatFromCircularOrbit(
  sat: Pick<SatelliteData, "altitude" | "inclination" | "startAngle">
): number {
  const inclinationRad = (sat.inclination * Math.PI) / 180;
  const angle = sat.startAngle;
  const x = Math.cos(angle);
  const y = Math.sin(angle) * Math.cos(inclinationRad);
  const z = Math.sin(angle) * Math.sin(inclinationRad);
  return Math.asin(Math.max(-1, Math.min(1, z))) * (180 / Math.PI);
}

/** Geodetic latitude (deg) on the same circular-orbit model after elapsed seconds from epoch. */
export function geodeticLatFromOrbitAtElapsedSeconds(
  altitudeKm: number,
  inclinationDeg: number,
  startAngleRad: number,
  elapsedSec: number
): number {
  const earthRadius = 6371;
  const orbitRadius = earthRadius + altitudeKm;
  const mu = 398600.4418;
  const orbitalPeriod = 2 * Math.PI * Math.sqrt(Math.pow(orbitRadius, 3) / mu);
  const angularVelocity = (2 * Math.PI) / orbitalPeriod;
  const inclinationRad = (inclinationDeg * Math.PI) / 180;
  const angle = startAngleRad + angularVelocity * elapsedSec;
  const x = Math.cos(angle);
  const y = Math.sin(angle) * Math.cos(inclinationRad);
  const z = Math.sin(angle) * Math.sin(inclinationRad);
  return Math.asin(Math.max(-1, Math.min(1, z))) * (180 / Math.PI);
}
