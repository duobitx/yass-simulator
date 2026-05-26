import { useEffect, useState } from "react";
import type { MutableRefObject } from "react";
import { X, Satellite, MapPin, Clock, Gauge, Radio, Link2, FileUp, FileDown, FileX } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { SseAgentFileEvent } from "@/lib/sse-types";

export interface SatelliteInfo {
  id: string;
  name: string;
  orbitType: "LEO" | "MEO" | "GEO";
  altitude: number;
  inclination: number;
  color: string;
  lat?: number;
  lon?: number;
  speed?: number;
  period?: number;
  connectedSatellites?: string[];
  connectedStations?: string[];
  agentEvents?: SseAgentFileEvent[];
}

function formatBytes(n?: number): string {
  if (n == null) return "";
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
  if (n < 1024 * 1024 * 1024) return `${(n / 1024 / 1024).toFixed(1)} MiB`;
  return `${(n / 1024 / 1024 / 1024).toFixed(2)} GiB`;
}

function formatTime(ts: string): string {
  try {
    const d = new Date(ts);
    return d.toLocaleTimeString();
  } catch {
    return ts;
  }
}

interface SatelliteInfoPopupProps {
  satellite: SatelliteInfo | null;
  onClose: () => void;
  position?: { x: number; y: number };
  eventsRef?: MutableRefObject<Record<string, SseAgentFileEvent[]>>;
}

const SatelliteInfoPopup = ({ satellite, onClose, position, eventsRef }: SatelliteInfoPopupProps) => {
  if (!satellite) return null;

  const getOrbitalPeriod = (altitude: number) => {
    const earthRadius = 6371;
    const orbitRadius = earthRadius + altitude;
    const mu = 398600.4418;
    return 2 * Math.PI * Math.sqrt(Math.pow(orbitRadius, 3) / mu);
  };

  const periodMinutes = Math.round(getOrbitalPeriod(satellite.altitude) / 60);
  const orbitalSpeed = (2 * Math.PI * (6371 + satellite.altitude)) / getOrbitalPeriod(satellite.altitude);

  const orbitTypeLabels = {
    LEO: "Low Earth Orbit",
    MEO: "Medium Earth Orbit",
    GEO: "Geostationary Orbit",
  };

  return (
    <div
      className="glass-card p-4 min-w-[280px] animate-scale-in"
    >
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-2">
          <div
            className="w-3 h-3 rounded-full"
            style={{ backgroundColor: satellite.color }}
          />
          <h3 className="font-semibold text-foreground">{satellite.name}</h3>
        </div>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 w-6 p-0"
          onClick={onClose}
        >
          <X className="h-4 w-4" />
        </Button>
      </div>

      <div className="space-y-3">
        <div className="flex items-center gap-2 text-sm">
          <Satellite className="h-4 w-4 text-muted-foreground" />
          <span className="text-muted-foreground">Type:</span>
          <span className="font-medium">{orbitTypeLabels[satellite.orbitType]}</span>
        </div>

        <div className="grid grid-cols-2 gap-3 text-sm">
          <div className="p-2 rounded bg-secondary/50">
            <p className="text-xs text-muted-foreground">Altitude</p>
            <p className="font-mono font-medium">{satellite.altitude.toLocaleString()} km</p>
          </div>
          <div className="p-2 rounded bg-secondary/50">
            <p className="text-xs text-muted-foreground">Inclination</p>
            <p className="font-mono font-medium">{satellite.inclination.toFixed(1)}°</p>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-3 text-sm">
          <div className="p-2 rounded bg-secondary/50">
            <div className="flex items-center gap-1 mb-1">
              <Clock className="h-3 w-3 text-muted-foreground" />
              <p className="text-xs text-muted-foreground">Orbital Period</p>
            </div>
            <p className="font-mono font-medium">{periodMinutes} min</p>
          </div>
          <div className="p-2 rounded bg-secondary/50">
            <div className="flex items-center gap-1 mb-1">
              <Gauge className="h-3 w-3 text-muted-foreground" />
              <p className="text-xs text-muted-foreground">Velocity</p>
            </div>
            <p className="font-mono font-medium">{orbitalSpeed.toFixed(2)} km/s</p>
          </div>
        </div>

        {satellite.lat !== undefined && satellite.lon !== undefined && (
          <div className="flex items-center gap-2 text-sm p-2 rounded bg-primary/10 border border-primary/20">
            <MapPin className="h-4 w-4 text-primary" />
            <div>
              <p className="text-xs text-muted-foreground">Current Position</p>
              <p className="font-mono text-primary">
                {satellite.lat.toFixed(2)}°{satellite.lat >= 0 ? "N" : "S"},{" "}
                {satellite.lon.toFixed(2)}°{satellite.lon >= 0 ? "E" : "W"}
              </p>
            </div>
          </div>
        )}

        {satellite.connectedSatellites && satellite.connectedSatellites.length > 0 && (
          <div className="flex items-center gap-2 text-sm p-2 rounded bg-secondary/50">
            <Link2 className="h-4 w-4 text-fuchsia-400" />
            <div>
              <p className="text-xs text-muted-foreground">Linked Satellite</p>
              <p className="font-medium text-fuchsia-300">{satellite.connectedSatellites[0]}</p>
            </div>
          </div>
        )}

        {satellite.connectedStations && satellite.connectedStations.length > 0 && (
          <div className="flex items-center gap-2 text-sm p-2 rounded bg-secondary/50">
            <Radio className="h-4 w-4 text-cyan-400" />
            <div>
              <p className="text-xs text-muted-foreground">Ground Station</p>
              <p className="font-medium text-cyan-300">{satellite.connectedStations[0]}</p>
            </div>
          </div>
        )}

        {(!satellite.connectedSatellites || satellite.connectedSatellites.length === 0) &&
         (!satellite.connectedStations || satellite.connectedStations.length === 0) && (
          <div className="text-xs text-muted-foreground italic p-2 rounded bg-secondary/50">
            No active links
          </div>
        )}

        <AgentEventsList fsNodeId={satellite.id} fallback={satellite.agentEvents} eventsRef={eventsRef} />
      </div>
    </div>
  );
};

interface AgentEventsListProps {
  fsNodeId: string;
  fallback?: SseAgentFileEvent[];
  eventsRef?: MutableRefObject<Record<string, SseAgentFileEvent[]>>;
}

const POLL_MS = 1000;

export const AgentEventsList = ({ fsNodeId, fallback, eventsRef }: AgentEventsListProps) => {
  const [events, setEvents] = useState<SseAgentFileEvent[]>(
    () => eventsRef?.current[fsNodeId] ?? fallback ?? []
  );
  useEffect(() => {
    if (!eventsRef) return;
    setEvents(eventsRef.current[fsNodeId] ?? []);
    const t = setInterval(() => {
      setEvents(eventsRef.current[fsNodeId] ?? []);
    }, POLL_MS);
    return () => clearInterval(t);
  }, [eventsRef, fsNodeId]);

  if (!events || events.length === 0) {
    return (
      <div className="pt-2 border-t border-border/50">
        <p className="text-xs text-muted-foreground uppercase tracking-wide mb-1">Agent events</p>
        <p className="text-xs text-muted-foreground italic p-2 rounded bg-secondary/50">
          No file events yet
        </p>
      </div>
    );
  }
  return (
    <div className="pt-2 border-t border-border/50">
      <p className="text-xs text-muted-foreground uppercase tracking-wide mb-1">
        Agent events ({events.length})
      </p>
      <div className="max-h-40 overflow-y-auto space-y-1">
        {events.map((e, i) => {
          const Icon = e.action === "PUT" ? FileUp : e.action === "RECEIVED" ? FileDown : FileX;
          const color =
            e.action === "PUT"
              ? "text-amber-400"
              : e.action === "RECEIVED"
              ? "text-emerald-400"
              : "text-rose-400";
          return (
            <div key={`${e.timestamp}-${i}`} className="flex items-start gap-2 text-xs p-1.5 rounded bg-secondary/40">
              <Icon className={`h-3 w-3 mt-0.5 ${color}`} />
              <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                  <span className={`font-mono font-medium ${color}`}>{e.action}</span>
                  <span className="text-muted-foreground">{formatTime(e.timestamp)}</span>
                  {e.contentSizeBytes ? (
                    <span className="ml-auto font-mono text-muted-foreground">{formatBytes(e.contentSizeBytes)}</span>
                  ) : null}
                </div>
                <p className="font-mono truncate text-foreground/80">{e.fileName}</p>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};

export default SatelliteInfoPopup;
