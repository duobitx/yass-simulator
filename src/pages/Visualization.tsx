import { useState, useCallback, useEffect, useRef, useMemo, Suspense, lazy } from "react";
import { Link, useParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  ArrowLeft,
  Satellite,
  Globe,
  Radio,
  Activity,
  Gauge,
  Play,
  Pause,
  Maximize,
  Minimize,
  Circle,
  Link2,
  Database,
  Clock,
  Loader2,
} from "lucide-react";
import SatelliteInfoPopup, { SatelliteInfo } from "@/components/visualization/SatelliteInfoPopup";
import GroundStationInfoPopup, { GroundStationInfo } from "@/components/visualization/GroundStationInfoPopup";
import SatelliteSearch from "@/components/visualization/SatelliteSearch";
import PassPrediction from "@/components/visualization/PassPrediction";
import esaLogo from "@/assets/esa-logo.svg";
import { getExperimentConfig } from "@/lib/experiment-configs";

// Lazy load CesiumScene
const CesiumScene = lazy(() => import("@/components/visualization/CesiumScene"));

const groundStations = [
  { name: "Darmstadt (ESOC)", lat: 49.87, lng: 8.63 },
  { name: "Kourou", lat: 5.16, lng: -52.65 },
  { name: "Perth", lat: -31.95, lng: 115.86 },
  { name: "Kiruna", lat: 67.86, lng: 20.22 },
  { name: "Maspalomas", lat: 27.76, lng: -15.58 },
  { name: "Redu", lat: 50.0, lng: 5.15 },
];

const SPEED_OPTIONS = [
  { label: "1x", value: 1, key: "1" },
  { label: "10x", value: 10, key: "2" },
  { label: "60x", value: 60, key: "3" },
  { label: "100x", value: 100, key: "4" },
];

const Visualization = () => {
  const { experimentId } = useParams<{ experimentId: string }>();
  const experimentConfig = useMemo(() => getExperimentConfig(experimentId), [experimentId]);
  const containerRef = useRef<HTMLDivElement>(null);
  const [showLEO, setShowLEO] = useState(true);
  const [showMEO, setShowMEO] = useState(true);
  const [showGEO, setShowGEO] = useState(true);
  const [showGroundStations, setShowGroundStations] = useState(true);
  const [showDataTransfer, setShowDataTransfer] = useState(true);
  const [showOrbits, setShowOrbits] = useState(true);
  const [showGroundLinks, setShowGroundLinks] = useState(true);
  const [useTLEData, setUseTLEData] = useState(true);
  const [simulationSpeed, setSimulationSpeed] = useState(60);
  const [isPaused, setIsPaused] = useState(false);
  const [selectedSatellite, setSelectedSatellite] = useState<SatelliteInfo | null>(null);
  const [selectedStation, setSelectedStation] = useState<GroundStationInfo | null>(null);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const [maxLinkDistance, setMaxLinkDistance] = useState(8000);

  const baseTime = useMemo(() => new Date(), []);

  // Keyboard shortcuts
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      switch (e.key) {
        case " ":
          e.preventDefault();
          setIsPaused((prev) => !prev);
          break;
        case "1": setSimulationSpeed(SPEED_OPTIONS[0].value); break;
        case "2": setSimulationSpeed(SPEED_OPTIONS[1].value); break;
        case "3": setSimulationSpeed(SPEED_OPTIONS[2].value); break;
        case "4": setSimulationSpeed(SPEED_OPTIONS[3].value); break;
        case "f":
        case "F": toggleFullscreen(); break;
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, []);

  useEffect(() => {
    const handleFullscreenChange = () => setIsFullscreen(!!document.fullscreenElement);
    document.addEventListener("fullscreenchange", handleFullscreenChange);
    return () => document.removeEventListener("fullscreenchange", handleFullscreenChange);
  }, []);

  const toggleFullscreen = useCallback(() => {
    if (!document.fullscreenElement) containerRef.current?.requestFullscreen();
    else document.exitFullscreen();
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

  const leoCount = experimentConfig.satellites.filter(s => s.orbitType === "LEO").length;
  const meoCount = experimentConfig.satellites.filter(s => s.orbitType === "MEO").length;
  const geoCount = experimentConfig.satellites.filter(s => s.orbitType === "GEO").length;

  const orbitStats = [
    { name: "LEO", color: "#4ade80", altitude: "200-2,000 km", satellites: leoCount, active: showLEO },
    { name: "MEO", color: "#facc15", altitude: "2,000-35,786 km", satellites: meoCount, active: showMEO },
    { name: "GEO", color: "#f97316", altitude: "35,786 km", satellites: geoCount, active: showGEO },
  ];

  return (
    <div ref={containerRef} className="h-screen bg-background flex flex-col overflow-hidden">
      {/* Header */}
      <header className="h-14 border-b border-border bg-card/80 backdrop-blur-xl sticky top-0 z-50">
        <div className="container mx-auto px-4 h-full flex items-center justify-between">
          <div className="flex items-center gap-4">
            <Link to="/dashboard">
              <Button variant="ghost" size="sm" className="gap-2">
                <ArrowLeft className="h-4 w-4" />
                Dashboard
              </Button>
            </Link>
            <div className="h-6 w-px bg-border" />
            <img src={esaLogo} alt="ESA" className="h-6 w-auto" />
            <span className="text-sm font-medium hidden md:block">
              {experimentConfig.name}
            </span>
          </div>

          <div className="flex items-center gap-2">

            <SatelliteSearch
              onSelectSatellite={handleSatelliteClick}
              showLEO={showLEO}
              showMEO={showMEO}
              showGEO={showGEO}
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
              Live
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

          {/* Orbit toggles */}
          <div className="space-y-4">
            <h3 className="text-sm font-semibold flex items-center gap-2">
              <Satellite className="h-4 w-4 text-primary" />
              Orbital Layers
            </h3>
            <div className="space-y-3">
              {orbitStats.map((orbit) => (
                <div key={orbit.name} className="flex items-center justify-between p-3 rounded-lg bg-secondary/50">
                  <div className="flex items-center gap-3">
                    <div className="w-3 h-3 rounded-full" style={{ backgroundColor: orbit.color }} />
                    <div>
                      <p className="text-sm font-medium">{orbit.name}</p>
                      <p className="text-xs text-muted-foreground">{orbit.altitude}</p>
                    </div>
                  </div>
                  <Switch
                    checked={orbit.name === "LEO" ? showLEO : orbit.name === "MEO" ? showMEO : showGEO}
                    onCheckedChange={(checked) => {
                      if (orbit.name === "LEO") setShowLEO(checked);
                      else if (orbit.name === "MEO") setShowMEO(checked);
                      else setShowGEO(checked);
                    }}
                  />
                </div>
              ))}
            </div>
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
                  <p className="text-xs text-muted-foreground">{experimentConfig.groundStations.length} ESA stations</p>
                </div>
              </div>
              <Switch checked={showGroundStations} onCheckedChange={setShowGroundStations} />
            </div>


                <div className="flex items-center justify-between p-3 rounded-lg bg-secondary/50">
                  <div className="flex items-center gap-3">
                    <Activity className="h-4 w-4 text-accent" />
                    <div>
                      <p className="text-sm font-medium">Data Transfer</p>
                      <p className="text-xs text-muted-foreground">Inter-satellite links</p>
                    </div>
                  </div>
                  <Switch checked={showDataTransfer} onCheckedChange={setShowDataTransfer} />
                </div>

                {showDataTransfer && (
                  <div className="p-3 rounded-lg bg-secondary/50 space-y-2">
                    <div className="flex items-center justify-between">
                      <p className="text-xs font-medium text-muted-foreground">Link Distance</p>
                      <span className="text-xs font-mono text-primary">{maxLinkDistance.toLocaleString()} km</span>
                    </div>
                    <input
                      type="range"
                      min={2000}
                      max={50000}
                      step={500}
                      value={maxLinkDistance}
                      onChange={(e) => setMaxLinkDistance(Number(e.target.value))}
                      className="w-full h-1.5 accent-primary cursor-pointer"
                    />
                    <div className="flex justify-between text-[10px] text-muted-foreground">
                      <span>2,000 km</span>
                      <span>50,000 km</span>
                    </div>
                  </div>
                )}

                <div className="flex items-center justify-between p-3 rounded-lg bg-secondary/50">
                  <div className="flex items-center gap-3">
                    <Link2 className="h-4 w-4 text-accent" />
                    <div>
                      <p className="text-sm font-medium">Ground Links</p>
                      <p className="text-xs text-muted-foreground">Station to satellite</p>
                    </div>
                  </div>
                  <Switch checked={showGroundLinks} onCheckedChange={setShowGroundLinks} />
                </div>

                <div className="flex items-center justify-between p-3 rounded-lg bg-secondary/50">
                  <div className="flex items-center gap-3">
                    <Circle className="h-4 w-4 text-accent" />
                    <div>
                      <p className="text-sm font-medium">Orbit Lines</p>
                      <p className="text-xs text-muted-foreground">Show orbit paths</p>
                    </div>
                  </div>
                  <Switch checked={showOrbits} onCheckedChange={setShowOrbits} />
                </div>

                <div className="flex items-center justify-between p-3 rounded-lg bg-secondary/50">
                  <div className="flex items-center gap-3">
                    <Database className="h-4 w-4 text-accent" />
                    <div>
                      <p className="text-sm font-medium">Real TLE Data</p>
                      <p className="text-xs text-muted-foreground">Use satellite.js</p>
                    </div>
                  </div>
                  <Switch checked={useTLEData} onCheckedChange={setUseTLEData} />
                </div>
          </div>

          {/* Pass Prediction */}
          {useTLEData && (
            <div className="space-y-4">
              <h3 className="text-sm font-semibold flex items-center gap-2">
                <Clock className="h-4 w-4 text-primary" />
                Pass Prediction
              </h3>
              <PassPrediction groundStations={groundStations} baseTime={baseTime} />
            </div>
          )}

          {/* Statistics */}
          <div className="space-y-4">
            <h3 className="text-sm font-semibold">Network Statistics</h3>
            <div className="grid grid-cols-2 gap-2">
              <div className="p-3 rounded-lg bg-secondary/50 text-center">
                <p className="text-2xl font-bold text-primary">{experimentConfig.satellites.length}</p>
                <p className="text-xs text-muted-foreground">Total Satellites</p>
              </div>
              <div className="p-3 rounded-lg bg-secondary/50 text-center">
                <p className="text-2xl font-bold text-accent">{experimentConfig.groundStations.length}</p>
                <p className="text-xs text-muted-foreground">Ground Stations</p>
              </div>
              <div className="p-3 rounded-lg bg-secondary/50 text-center">
                <p className="text-2xl font-bold text-esa-success">98.7%</p>
                <p className="text-xs text-muted-foreground">Uptime</p>
              </div>
              <div className="p-3 rounded-lg bg-secondary/50 text-center">
                <p className="text-2xl font-bold text-esa-warning">124ms</p>
                <p className="text-xs text-muted-foreground">Avg Latency</p>
              </div>
            </div>
          </div>

          {/* Simulation Speed */}
          <div className="space-y-4">
            <h3 className="text-sm font-semibold flex items-center gap-2">
              <Gauge className="h-4 w-4 text-primary" />
              Simulation Speed
            </h3>
            <div className="flex items-center gap-2">
              <Button variant="outline" size="sm" className="h-9 w-9 p-0" onClick={() => setIsPaused(!isPaused)}>
                {isPaused ? <Play className="h-4 w-4" /> : <Pause className="h-4 w-4" />}
              </Button>
              <div className="flex flex-wrap gap-1 flex-1">
                {SPEED_OPTIONS.map((option) => (
                  <Button
                    key={option.value}
                    variant={simulationSpeed === option.value ? "default" : "outline"}
                    size="sm"
                    className="flex-1 min-w-[40px] h-9"
                    onClick={() => setSimulationSpeed(option.value)}
                    disabled={isPaused}
                  >
                    {option.label}
                  </Button>
                ))}
              </div>
            </div>
            {isPaused && <p className="text-xs text-esa-warning">Simulation paused</p>}
          </div>

          {/* Keyboard Shortcuts */}
          <div className="mt-auto space-y-2">
            <h3 className="text-sm font-semibold">Keyboard Shortcuts</h3>
            <div className="text-xs text-muted-foreground space-y-1">
              <p>• <kbd className="px-1 py-0.5 bg-secondary rounded text-[10px]">Space</kbd> Pause/Play</p>
              <p>• <kbd className="px-1 py-0.5 bg-secondary rounded text-[10px]">1-4</kbd> Change speed</p>
              <p>• <kbd className="px-1 py-0.5 bg-secondary rounded text-[10px]">F</kbd> Toggle fullscreen</p>
              <p>• Click + drag to rotate</p>
              <p>• Scroll to zoom</p>
            </div>
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
                  showLEO={showLEO}
                  showMEO={showMEO}
                  showGEO={showGEO}
                  showGroundStations={showGroundStations}
                  showDataTransfer={showDataTransfer}
                  showGroundLinks={showGroundLinks}
                  showOrbits={showOrbits}
                  showTrails={false}
                  simulationSpeed={simulationSpeed}
                  isPaused={isPaused}
                  maxLinkDistance={maxLinkDistance}
                  satellites={experimentConfig.satellites}
                  groundStationsList={experimentConfig.groundStations}
                  onSatelliteClick={handleSatelliteClick}
                  onGroundStationClick={handleGroundStationClick}
                />
              </Suspense>
          </div>

          {/* Info popups */}
          {selectedSatellite && (
            <div className="absolute top-4 right-4 z-10">
              <SatelliteInfoPopup satellite={selectedSatellite} onClose={handleClosePopup} />
            </div>
          )}
          {selectedStation && (
            <div className="absolute top-4 right-4 z-10">
              <GroundStationInfoPopup station={selectedStation} onClose={handleClosePopup} />
            </div>
          )}

          {/* Mobile controls overlay */}
          <div className="absolute bottom-4 left-4 right-4 lg:hidden">
            <div className="glass-card p-3 flex items-center justify-center gap-4">
              <div className="flex items-center gap-2">
                <Label htmlFor="leo-mobile" className="text-xs">LEO</Label>
                <Switch id="leo-mobile" checked={showLEO} onCheckedChange={setShowLEO} />
              </div>
              <div className="flex items-center gap-2">
                <Label htmlFor="meo-mobile" className="text-xs">MEO</Label>
                <Switch id="meo-mobile" checked={showMEO} onCheckedChange={setShowMEO} />
              </div>
              <div className="flex items-center gap-2">
                <Label htmlFor="geo-mobile" className="text-xs">GEO</Label>
                <Switch id="geo-mobile" checked={showGEO} onCheckedChange={setShowGEO} />
              </div>
            </div>
          </div>
        </main>
      </div>
    </div>
  );
};

export default Visualization;
