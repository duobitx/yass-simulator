import { Color } from "cesium";
import type { GroundStationData, SatelliteData } from "@/lib/experiment-configs";
import type { SsePositionEvent } from "@/lib/sse-types";
import { isGroundStationEvent } from "@/lib/sse-types";

function colorForSource(id: string): Color {
  let h = 0;
  for (let i = 0; i < id.length; i++) h = id.charCodeAt(i) + ((h << 5) - h);
  const hue = (Math.abs(h) % 360) / 360;
  return Color.fromHsl(hue, 0.85, 0.55, 1);
}

function classifyOrbitFromAltKm(altKm: number): "LEO" | "MEO" | "GEO" {
  if (altKm < 2000) return "LEO";
  if (altKm < 35000) return "MEO";
  return "GEO";
}

export function formatTrackLabel(source: string): string {
  return source
    .replace(/^estrack-/, "")
    .split("-")
    .filter(Boolean)
    .map((w) => w.charAt(0).toUpperCase() + w.slice(1))
    .join(" ");
}

export function satelliteFromSseTrack(id: string, ev: SsePositionEvent): SatelliteData {
  const orbitType = classifyOrbitFromAltKm(ev.Alt);
  return {
    id,
    name: formatTrackLabel(id),
    orbitType,
    altitude: Math.round(ev.Alt),
    inclination: orbitType === "GEO" ? 0.1 : 53,
    color: colorForSource(id),
    startAngle: 0,
  };
}

export function groundFromSseTrack(id: string, ev: SsePositionEvent): GroundStationData {
  return {
    id,
    name: formatTrackLabel(id),
    lat: ev.Lat,
    lon: ev.Lng,
  };
}

export function listsFromSseTracks(
  tracks: Record<string, SsePositionEvent>,
  sourceSignature: string
): { satellites: SatelliteData[]; groundStations: GroundStationData[] } {
  if (!sourceSignature) return { satellites: [], groundStations: [] };
  const satellites: SatelliteData[] = [];
  const groundStations: GroundStationData[] = [];
  for (const id of sourceSignature.split(",").filter(Boolean)) {
    const ev = tracks[id];
    if (!ev) continue;
    if (isGroundStationEvent(ev)) groundStations.push(groundFromSseTrack(id, ev));
    else satellites.push(satelliteFromSseTrack(id, ev));
  }
  return { satellites, groundStations };
}
