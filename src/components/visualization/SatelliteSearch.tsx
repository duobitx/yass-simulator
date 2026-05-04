import { useMemo, useState } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Search, X, Satellite } from "lucide-react";
import { SatelliteInfo } from "./SatelliteInfoPopup";
import { ORBIT_LAYERS } from "@/lib/orbit-layers";

export interface SatelliteSearchItem {
  id: string;
  name: string;
  lat: number;
  lon?: number;
  color: string;
  orbitType: "LEO" | "MEO" | "GEO";
  altitude: number;
  inclination: number;
}

interface SatelliteSearchProps {
  satellites: SatelliteSearchItem[];
  orbitLayerVisibility: Record<string, boolean>;
  onSelectSatellite: (satellite: SatelliteInfo) => void;
}

const SatelliteSearch = ({
  satellites,
  orbitLayerVisibility,
  onSelectSatellite,
}: SatelliteSearchProps) => {
  const [searchQuery, setSearchQuery] = useState("");
  const [isOpen, setIsOpen] = useState(false);

  const filteredSatellites = useMemo(() => {
    const q = searchQuery.trim().toLowerCase();
    return satellites.filter((sat) => {
      if (orbitLayerVisibility[sat.orbitType] === false) return false;
      if (!q) return true;
      const layer = ORBIT_LAYERS.find((l) => l.id === sat.orbitType);
      const layerHint = layer ? `${layer.label} ${layer.rangeHint}`.toLowerCase() : "";
      return (
        sat.name.toLowerCase().includes(q) ||
        sat.id.toLowerCase().includes(q) ||
        sat.orbitType.toLowerCase().includes(q) ||
        layerHint.includes(q) ||
        `${sat.lat.toFixed(1)}°`.includes(q)
      );
    });
  }, [satellites, searchQuery, orbitLayerVisibility]);

  const groupedSatellites = useMemo(() => {
    const groups = new Map<string, SatelliteSearchItem[]>();
    for (const layer of ORBIT_LAYERS) {
      groups.set(layer.id, []);
    }
    filteredSatellites.forEach((sat) => {
      groups.get(sat.orbitType)?.push(sat);
    });
    return ORBIT_LAYERS.map((layer) => ({
      layer,
      satellites: groups.get(layer.id) ?? [],
    })).filter((g) => g.satellites.length > 0);
  }, [filteredSatellites]);

  const handleSelect = (sat: SatelliteSearchItem) => {
    onSelectSatellite({
      id: sat.id,
      name: sat.name,
      orbitType: sat.orbitType,
      altitude: sat.altitude,
      inclination: sat.inclination,
      color: sat.color,
      lat: sat.lat,
      lon: sat.lon,
    });
    setIsOpen(false);
    setSearchQuery("");
  };

  return (
    <div className="relative">
      <Button
        variant="outline"
        size="sm"
        className="gap-2"
        onClick={() => setIsOpen(!isOpen)}
      >
        <Search className="h-4 w-4" />
        <span className="hidden sm:inline">Search Satellites</span>
      </Button>

      {isOpen && (
        <div className="absolute top-full left-0 mt-2 w-72 bg-card border border-border rounded-lg shadow-xl z-50 overflow-hidden">
          <div className="p-3 border-b border-border">
            <div className="relative">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search by name, LEO/MEO/GEO, latitude…"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9 pr-9"
                autoFocus
              />
              {searchQuery && (
                <Button
                  variant="ghost"
                  size="sm"
                  className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6 p-0"
                  onClick={() => setSearchQuery("")}
                >
                  <X className="h-3 w-3" />
                </Button>
              )}
            </div>
          </div>

          <div className="max-h-64 overflow-y-auto p-2">
            {satellites.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-4">
                No satellites in this experiment
              </p>
            ) : filteredSatellites.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-4">
                No satellites found
              </p>
            ) : (
              groupedSatellites.map(({ layer, satellites: groupSats }) => (
                <div key={layer.id} className="mb-2">
                  <p className="text-xs font-semibold text-muted-foreground px-2 py-1">
                    {layer.label} <span className="font-normal opacity-80">({layer.rangeHint})</span>
                  </p>
                  <div className="space-y-1">
                    {groupSats.map((sat) => (
                      <button
                        key={sat.id}
                        type="button"
                        className="w-full flex items-center gap-3 px-3 py-2 rounded-md hover:bg-secondary/80 transition-colors text-left"
                        onClick={() => handleSelect(sat)}
                      >
                        <div
                          className="w-2.5 h-2.5 rounded-full shrink-0"
                          style={{ backgroundColor: sat.color }}
                        />
                        <Satellite className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
                        <span className="text-sm font-medium truncate">{sat.name}</span>
                        <span className="text-xs text-muted-foreground ml-auto shrink-0 tabular-nums">
                          {sat.orbitType}
                        </span>
                      </button>
                    ))}
                  </div>
                </div>
              ))
            )}
          </div>

          <div className="p-2 border-t border-border bg-secondary/30">
            <p className="text-xs text-muted-foreground text-center">
              {satellites.length === 0
                ? "—"
                : `${filteredSatellites.length} of ${satellites.length} satellite${satellites.length !== 1 ? "s" : ""}`}
            </p>
          </div>
        </div>
      )}
    </div>
  );
};

export default SatelliteSearch;
