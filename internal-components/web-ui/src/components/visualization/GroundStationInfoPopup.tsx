import { X, Radio, MapPin, Satellite } from "lucide-react";
import type { MutableRefObject } from "react";
import { Button } from "@/components/ui/button";
import type { SseAgentFileEvent } from "@/lib/sse-types";
import type { ExperimentClockAnchor } from "@/hooks/useSatelliteSse";
import { AgentEventsList } from "./SatelliteInfoPopup";

export interface GroundStationInfo {
  id: string;
  name: string;
  lat: number;
  lon: number;
  connectedSatellite?: string;
  agentEvents?: SseAgentFileEvent[];
}

interface GroundStationInfoPopupProps {
  station: GroundStationInfo | null;
  onClose: () => void;
  eventsRef?: MutableRefObject<Record<string, SseAgentFileEvent[]>>;
  expClockRef?: MutableRefObject<ExperimentClockAnchor | null>;
}

const GroundStationInfoPopup = ({ station, onClose, eventsRef, expClockRef }: GroundStationInfoPopupProps) => {
  if (!station) return null;

  return (
    <div className="glass-card p-4 min-w-[260px] animate-scale-in">
      <div className="flex items-start justify-between mb-3">
        <div className="flex items-center gap-2">
          <Radio className="h-4 w-4 text-cyan-400" />
          <h3 className="font-semibold text-foreground">{station.name}</h3>
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
          <Radio className="h-4 w-4 text-muted-foreground" />
          <span className="text-muted-foreground">Type:</span>
          <span className="font-medium">ESA Ground Station</span>
        </div>

        <div className="flex items-center gap-2 text-sm p-2 rounded bg-primary/10 border border-primary/20">
          <MapPin className="h-4 w-4 text-primary" />
          <div>
            <p className="text-xs text-muted-foreground">Location</p>
            <p className="font-mono text-primary">
              {Math.abs(station.lat).toFixed(2)}°{station.lat >= 0 ? "N" : "S"},{" "}
              {Math.abs(station.lon).toFixed(2)}°{station.lon >= 0 ? "E" : "W"}
            </p>
          </div>
        </div>

        {station.connectedSatellite ? (
          <div className="flex items-center gap-2 text-sm p-2 rounded bg-secondary/50">
            <Satellite className="h-4 w-4 text-accent" />
            <div>
              <p className="text-xs text-muted-foreground">Connected Satellite</p>
              <p className="font-medium text-accent">{station.connectedSatellite}</p>
            </div>
          </div>
        ) : (
          <div className="text-xs text-muted-foreground italic p-2 rounded bg-secondary/50">
            No satellite in view
          </div>
        )}

        <AgentEventsList fsNodeId={station.id} fallback={station.agentEvents} eventsRef={eventsRef} expClockRef={expClockRef} />
      </div>
    </div>
  );
};

export default GroundStationInfoPopup;
