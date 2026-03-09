import { useState, useMemo } from "react";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Search, X, Satellite } from "lucide-react";
import { SatelliteInfo } from "./SatelliteInfoPopup";

interface SatelliteData {
  id: string;
  name: string;
  orbitType: "LEO" | "MEO" | "GEO";
  color: string;
}

interface SatelliteSearchProps {
  onSelectSatellite: (satellite: SatelliteInfo) => void;
  showLEO: boolean;
  showMEO: boolean;
  showGEO: boolean;
}

// Generate all satellites data
const generateSatellites = (): SatelliteData[] => {
  const satellites: SatelliteData[] = [];
  
  // LEO satellites
  for (let i = 0; i < 8; i++) {
    satellites.push({
      id: `LEO-${i}`,
      name: `LEO-${i + 1}`,
      orbitType: "LEO",
      color: "#4ade80",
    });
  }
  
  // MEO satellites
  for (let i = 0; i < 4; i++) {
    satellites.push({
      id: `MEO-${i}`,
      name: `MEO-${i + 1}`,
      orbitType: "MEO",
      color: "#facc15",
    });
  }
  
  // GEO satellites
  for (let i = 0; i < 3; i++) {
    satellites.push({
      id: `GEO-${i}`,
      name: `GEO-${i + 1}`,
      orbitType: "GEO",
      color: "#f97316",
    });
  }
  
  return satellites;
};

const ORBIT_DETAILS = {
  LEO: { altitude: 550, inclination: 53 },
  MEO: { altitude: 20200, inclination: 55 },
  GEO: { altitude: 35786, inclination: 0 },
};

const SatelliteSearch = ({ onSelectSatellite, showLEO, showMEO, showGEO }: SatelliteSearchProps) => {
  const [searchQuery, setSearchQuery] = useState("");
  const [isOpen, setIsOpen] = useState(false);

  const allSatellites = useMemo(() => generateSatellites(), []);

  const filteredSatellites = useMemo(() => {
    return allSatellites.filter((sat) => {
      // Filter by visibility
      if (sat.orbitType === "LEO" && !showLEO) return false;
      if (sat.orbitType === "MEO" && !showMEO) return false;
      if (sat.orbitType === "GEO" && !showGEO) return false;

      // Filter by search query
      if (searchQuery) {
        const query = searchQuery.toLowerCase();
        return (
          sat.name.toLowerCase().includes(query) ||
          sat.orbitType.toLowerCase().includes(query)
        );
      }
      return true;
    });
  }, [allSatellites, searchQuery, showLEO, showMEO, showGEO]);

  const handleSelect = (sat: SatelliteData) => {
    const details = ORBIT_DETAILS[sat.orbitType];
    onSelectSatellite({
      id: sat.id,
      name: sat.name,
      orbitType: sat.orbitType,
      altitude: details.altitude,
      inclination: details.inclination,
      color: sat.color,
    });
    setIsOpen(false);
    setSearchQuery("");
  };

  const groupedSatellites = useMemo(() => {
    const groups: Record<string, SatelliteData[]> = {
      LEO: [],
      MEO: [],
      GEO: [],
    };
    filteredSatellites.forEach((sat) => {
      groups[sat.orbitType].push(sat);
    });
    return groups;
  }, [filteredSatellites]);

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
                placeholder="Search by name or orbit type..."
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
            {filteredSatellites.length === 0 ? (
              <p className="text-sm text-muted-foreground text-center py-4">
                No satellites found
              </p>
            ) : (
              Object.entries(groupedSatellites).map(([orbitType, satellites]) => {
                if (satellites.length === 0) return null;
                return (
                  <div key={orbitType} className="mb-2">
                    <p className="text-xs font-semibold text-muted-foreground px-2 py-1">
                      {orbitType} Orbit
                    </p>
                    <div className="space-y-1">
                      {satellites.map((sat) => (
                        <button
                          key={sat.id}
                          className="w-full flex items-center gap-3 px-3 py-2 rounded-md hover:bg-secondary/80 transition-colors text-left"
                          onClick={() => handleSelect(sat)}
                        >
                          <div
                            className="w-2.5 h-2.5 rounded-full"
                            style={{ backgroundColor: sat.color }}
                          />
                          <Satellite className="h-3.5 w-3.5 text-muted-foreground" />
                          <span className="text-sm font-medium">{sat.name}</span>
                        </button>
                      ))}
                    </div>
                  </div>
                );
              })
            )}
          </div>

          <div className="p-2 border-t border-border bg-secondary/30">
            <p className="text-xs text-muted-foreground text-center">
              {filteredSatellites.length} satellite{filteredSatellites.length !== 1 ? "s" : ""} found
            </p>
          </div>
        </div>
      )}
    </div>
  );
};

export default SatelliteSearch;
