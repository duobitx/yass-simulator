import { useState, useCallback, useEffect, useRef, useMemo, Suspense, lazy } from "react";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import {
  Globe,
  Radio,
  Activity,
  Maximize,
  Minimize,
  Circle,
  Loader2,
} from "lucide-react";
import SatelliteInfoPopup, { SatelliteInfo } from "@/components/visualization/SatelliteInfoPopup";
import GroundStationInfoPopup, { GroundStationInfo } from "@/components/visualization/GroundStationInfoPopup";
import SatelliteSearch, { type SatelliteSearchItem } from "@/components/visualization/SatelliteSearch";
import esaLogo from "@/assets/esa-logo.svg";
import { useSatelliteSse } from "@/hooks/useSatelliteSse";
import { sseEventsUrl } from "@/lib/sse-types";
import { listsFromSseTracks } from "@/lib/sse-track-utils";
import { approxGeodeticLatFromCircularOrbit } from "@/lib/orbit-layers";

const ALL_LAYERS_VISIBLE: Record<string, boolean> = {};

function useExperimentName(): string | null {
  const [name, setName] = useState<string | null>(null);
  useEffect(() => {
    const ac = new AbortController();
    fetch("/experiment-executor/info", { signal: ac.signal })
      .then((r) => (r.ok ? r.json() : null))
      .then((data: { name?: string } | null) => {
        if (data && typeof data.name === "string") setName(data.name);
      })
      .catch(() => { /* ignore — header falls back to a placeholder */ });
    return () => ac.abort();
  }, []);
  return name;
}

const CesiumScene = lazy(() => import("@/components/visualization/CesiumScene"));

const Visualization = () => {
  const sse = useSatelliteSse(sseEventsUrl(), true);
  const experimentName = useExperimentName();

  const { sceneSatellites, sceneGroundStations } = useMemo(() => {
    if (!sse.sourceSignature) {
      return { sceneSatellites: [], sceneGroundStations: [] };
    }
    const live = listsFromSseTracks(sse.tracksRef.current, sse.sourceSignature);
    return {
      sceneSatellites: live.satellites,
      sceneGroundStations: live.groundStations,
    };
  }, [sse.sourceSignature, sse.tracksRef]);

  const liveMode = Boolean(sse.sourceSignature);

  const containerRef = useRef<HTMLDivElement>(null);
  const [showGroundStations, setShowGroundStations] = useState(true);
  const [showOrbits, setShowOrbits] = useState(false);
  const [selectedSatellite, setSelectedSatellite] = useState<SatelliteInfo | null>(null);
  const [selectedStation, setSelectedStation] = useState<GroundStationInfo | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);

  const toggleFullscreen = useCallback(() => {
    if (!document.fullscreenElement) containerRef.current?.requestFullscreen();
    else document.exitFullscreen();
  }, []);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.key === "f" || e.key === "F") toggleFullscreen();
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [toggleFullscreen]);

  useEffect(() => {
    const handleFullscreenChange = () => setIsFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => document.removeEventListener("fullscreenchange", handleFullscreenChange);
  }, []);

  const handleSatelliteClick = useCallback((satellite: SatelliteInfo) => {
    setSelectedSatellite(satellite);
    setSelectedStation(null);
  }, []);

  const handleGroundStationClick = useCallback((station: GroundStationInfo) => {
    setSelectedStation(station);
    setSelectedSatellite(null);
  }, []);

  const handleClosePopup = useCallback(() => {
    setSelectedSatellite(null);
    setSelectedStation(null);
  }, []);

  const latLonForSceneSat = useCallback(
    (s: (typeof sceneSatellites)[number]): { lat: number; lon?: number } => {
      const ev = sse.tracks[s.id];
      if (ev) return { lat: ev.Lat, lon: ev.Lng };
      return { lat: approxGeodeticLatFromCircularOrbit(s) };
    },
    [sse.tracks]
  );

  const searchSatellites = useMemo((): SatelliteSearchItem[] => {
    return sceneSatellites.flatMap((s) => {
      if (!sse.tracks[s.id]) return [];
      const { lat, lon } = latLonForSceneSat(s);
      return [
        {
          id: s.id,
          name: s.name,
          lat,
          lon,
          color: s.color.toCssColorString(),
          orbitType: s.orbitType,
          altitude: s.altitude,
          inclination: s.inclination,
        },
      ];
    });
  }, [sceneSatellites, latLonForSceneSat, sse.tracks]);

  return (
    <div ref={containerRef} className="h-screen bg-background flex flex-col overflow-hidden">
      {/* Header */}
      <header className="h-14 border-b border-border bg-card/80 backdrop-blur-xl sticky top-0 z-50">
        <div className="container mx-auto px-4 h-full flex items-center justify-between">
          <div className="flex items-center gap-4">
            <img src={esaLogo} alt="ESA" className="h-6 w-auto" />
            <span className="text-sm font-medium hidden md:block">
              Experiment: {experimentName ?? "…"}
            </span>
          </div>

          <div className="flex items-center gap-2">
            <SatelliteSearch
              satellites={searchSatellites}
              orbitLayerVisibility={ALL_LAYERS_VISIBLE}
              onSelectSatellite={handleSatelliteClick}
            />

            <Button
              variant="outline"
              size="sm"
              className="gap-1.5"
              onClick={toggleFullscreen}
            >
              {isFullscreen ? <Minimize className="h-3.5 w-3.5" /> : <Maximize className="h-3.5 w-3.5" />}
              <span className="hidden sm:inline">{isFullscreen ? "Exit" : "Fullscreen"}</span>
            </Button>
            <Badge variant="outline" className="gap-1.5 bg-esa-success/20 text-esa-success border-esa-success/30">
              <Activity className="h-3 w-3" />
              {sse.status === "live" ? "Live" : sse.status === "connecting" ? "Connecting…" : sse.status === "error" ? "Stream error" : "Idle"}
            </Badge>
          </div>
        </div>
      </header>

      {/* Main content */}
      <div className="flex-1 flex overflow-hidden">
        {/* Sidebar controls */}
        <aside className="w-72 border-r border-border bg-card/50 p-4 flex flex-col gap-6 hidden lg:flex overflow-y-auto flex-shrink-0">
          <div className="p-3 rounded-lg bg-primary/10 border border-primary/20">
            <p className="text-xs font-medium text-primary">Cesium Globe View</p>
            <p className="text-xs text-muted-foreground mt-1">
              Visualization powered by CesiumJS. Use the scene mode picker to switch between 3D, 2D, and Columbus views.
            </p>
          </div>

          {/* Ground Infrastructure */}
          <div className="space-y-4">
            <h3 className="text-sm font-semibold flex items-center gap-2">
              <Globe className="h-4 w-4 text-primary" />
              Infrastructure
            </h3>

            <div className="flex items-center justify-between p-3 rounded-lg bg-secondary/50">
              <div className="flex items-center gap-3">
                <Radio className="h-4 w-4 text-accent" />
                <div>
                  <p className="text-sm font-medium">Ground Stations</p>
                  <p className="text-xs text-muted-foreground">
                    {sceneGroundStations.length} from event stream
                  </p>
                </div>
              </div>
              <Switch checked={showGroundStations} onCheckedChange={setShowGroundStations} />
            </div>

            <div className="flex items-center justify-between p-3 rounded-lg bg-secondary/50">
              <div className="flex items-center gap-3">
                <Circle className="h-4 w-4 text-accent" />
                <div>
                  <p className="text-sm font-medium">Orbit Lines</p>
                  <p className="text-xs text-muted-foreground">
                    Approximate rings from stream altitude / regime
                  </p>
                </div>
              </div>
              <Switch checked={showOrbits} onCheckedChange={setShowOrbits} />
            </div>
          </div>

          {/* Legend */}
          <div className="space-y-2">
            <h3 className="text-sm font-semibold">Legend</h3>
            <div className="p-3 rounded-lg bg-secondary/50 space-y-2">
              <p className="text-xs text-muted-foreground">Visibility line bandwidth</p>
              {[
                { c: "#00ff66", l: "≥ 1 Gbit/s" },
                { c: "#22cc66", l: "≥ 50 Mbit/s" },
                { c: "#ffd84d", l: "≥ 5 Mbit/s" },
                { c: "#ff9933", l: "≥ 500 kbit/s" },
                { c: "#ff3355", l: "< 500 kbit/s" },
              ].map(({ c, l }) => (
                <div key={c} className="flex items-center gap-2">
                  <svg width="36" height="6" viewBox="0 0 36 6" aria-hidden>
                    <line x1="0" y1="3" x2="36" y2="3" stroke={c} strokeWidth="2" strokeDasharray="6 4" />
                  </svg>
                  <span className="text-xs">{l}</span>
                </div>
              ))}
              <div className="flex items-center gap-2 pt-1 border-t border-border/40">
                <svg width="36" height="8" viewBox="0 0 36 8" aria-hidden>
                  <circle cx="18" cy="4" r="3" fill="#22cc66" stroke="#ffffff80" />
                </svg>
                <span className="text-xs">packet flow</span>
              </div>
            </div>
          </div>

          {/* Statistics */}
          <div className="space-y-4">
            <h3 className="text-sm font-semibold">Network Statistics</h3>
            <div className="grid grid-cols-2 gap-2">
              <div className="p-3 rounded-lg bg-secondary/50 text-center">
                <p className="text-2xl font-bold text-primary">{sceneSatellites.length}</p>
                <p className="text-xs text-muted-foreground">Satellites</p>
              </div>
              <div className="p-3 rounded-lg bg-secondary/50 text-center">
                <p className="text-2xl font-bold text-accent">{sceneGroundStations.length}</p>
                <p className="text-xs text-muted-foreground">Ground Stations</p>
              </div>
            </div>
          </div>

          <div className="mt-auto space-y-4">
            {/* Keyboard Shortcuts */}
            <div className="space-y-2">
              <h3 className="text-sm font-semibold">Keyboard Shortcuts</h3>
              <div className="text-xs text-muted-foreground space-y-1">
                <p>• <kbd className="px-1 py-0.5 bg-secondary rounded text-[10px]">F</kbd> Toggle fullscreen</p>
                <p>• Click + drag to rotate</p>
                <p>• Scroll to zoom</p>
              </div>
            </div>
            <p className="hidden text-[10px] text-muted-foreground text-center">
              Created by duobit.pl for ESA.
            </p>
          </div>
        </aside>

        {/* Viewer */}
        <main className="flex-1 relative">
          <div className="absolute inset-0">
              <Suspense
                fallback={
                  <div className="absolute inset-0 flex items-center justify-center bg-background">
                    <div className="flex flex-col items-center gap-4">
                      <Loader2 className="h-8 w-8 animate-spin text-primary" />
                      <p className="text-muted-foreground">Loading CesiumJS...</p>
                    </div>
                  </div>
                }
              >
                <CesiumScene
                  orbitLayerVisibility={ALL_LAYERS_VISIBLE}
                  showGroundStations={showGroundStations}
                  showOrbits={showOrbits}
                  satellites={sceneSatellites}
                  groundStationsList={sceneGroundStations}
                  liveMode={liveMode}
                  liveTracksRef={sse.tracksRef}
                  liveUsageRef={sse.usageRef}
                  liveEventsRef={sse.eventsRef}
                  onSatelliteClick={handleSatelliteClick}
                  onGroundStationClick={handleGroundStationClick}
                />
              </Suspense>
          </div>

          {/* Info popups */}
          {selectedSatellite && (
            <div className="absolute top-4 right-4 z-10">
              <SatelliteInfoPopup satellite={selectedSatellite} onClose={handleClosePopup} eventsRef={sse.eventsRef} />
            </div>
          )}
          {selectedStation && (
            <div className="absolute top-4 right-4 z-10">
              <GroundStationInfoPopup station={selectedStation} onClose={handleClosePopup} eventsRef={sse.eventsRef} />
            </div>
          )}

        </main>
      </div>
    </div>
  );
};

export default Visualization;
