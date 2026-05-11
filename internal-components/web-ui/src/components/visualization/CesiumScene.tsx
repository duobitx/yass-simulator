import { useEffect, useRef, useState, useCallback } from "react";
import type { MutableRefObject } from "react";
import {
  ScreenSpaceEventHandler,
  ScreenSpaceEventType,
  defined,
  Viewer,
  Ion,
  Cartesian2,
  Cartesian3,
  Color,
  JulianDate,
  SampledPositionProperty,
  PolylineGlowMaterialProperty,
  PolylineDashMaterialProperty,
  ClockRange,
  ClockStep,
  IonImageryProvider,
  Ellipsoid,
  Cartographic,
  CallbackProperty,
  VelocityVectorProperty,
  Transforms,
  Matrix4,
  Event as CesiumEvent,
  Entity,
} from "cesium";
import "cesium/Build/Cesium/Widgets/widgets.css";

import type { SsePositionEvent } from "@/lib/sse-types";

// Cesium Ion access token
Ion.defaultAccessToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJqdGkiOiJlZWViZWY4NS1kY2ViLTRjNmItYjM2OS00NWY4MmRjZDY1YTUiLCJpZCI6Mzc5MDgzLCJpYXQiOjE3Njg0ODA2OTN9.xaHKt0sIqM-7mTqizQGILb0yoRGBYSZ9u9zaEiDCaLM";

interface SatelliteClickInfo {
  id: string;
  name: string;
  orbitType: "LEO" | "MEO" | "GEO";
  altitude: number;
  inclination: number;
  color: string;
  connectedSatellites?: string[];
  connectedStations?: string[];
}

interface GroundStationClickInfo {
  id: string;
  name: string;
  lat: number;
  lon: number;
  connectedSatellite?: string;
}

interface CesiumSceneProps {
  orbitLayerVisibility: Record<string, boolean>;
  showGroundStations: boolean;
  showOrbits: boolean;
  showDataTransfer?: boolean;
  showGroundLinks?: boolean;
  showTrails?: boolean;
  simulationSpeed?: number;
  isPaused?: boolean;
  maxLinkDistance?: number;
  satellites?: SatelliteData[];
  groundStationsList?: GroundStationData[];
  liveMode?: boolean;
  liveTracksRef?: MutableRefObject<Record<string, SsePositionEvent>>;
  onSatelliteClick?: (satellite: SatelliteClickInfo) => void;
  onGroundStationClick?: (station: GroundStationClickInfo) => void;
}

interface SatelliteData {
  id: string;
  name: string;
  orbitType: "LEO" | "MEO" | "GEO";
  altitude: number;
  inclination: number;
  color: Color;
  startAngle: number;
}

interface GroundStationData {
  id: string;
  name: string;
  lat: number;
  lon: number;
}

const createOrbitPath = (
  altitude: number,
  inclination: number,
  startAngle: number,
  startTime: JulianDate,
  duration: number
): SampledPositionProperty => {
  const property = new SampledPositionProperty();
  const earthRadius = 6371;
  const orbitRadius = earthRadius + altitude;
  const mu = 398600.4418;
  const orbitalPeriod = 2 * Math.PI * Math.sqrt(Math.pow(orbitRadius, 3) / mu);
  const numSamples = 360;
  const inclinationRad = (inclination * Math.PI) / 180;

  for (let i = 0; i <= numSamples; i++) {
    const timeOffset = (i / numSamples) * duration;
    const time = JulianDate.addSeconds(startTime, timeOffset, new JulianDate());
    const angularVelocity = (2 * Math.PI) / orbitalPeriod;
    const angle = startAngle + angularVelocity * timeOffset;
    const x = Math.cos(angle);
    const y = Math.sin(angle) * Math.cos(inclinationRad);
    const z = Math.sin(angle) * Math.sin(inclinationRad);
    const lon = Math.atan2(y, x) * (180 / Math.PI);
    const lat = Math.asin(z) * (180 / Math.PI);
    const position = Cartesian3.fromDegrees(lon, lat, altitude * 1000);
    property.addSample(time, position);
  }
  return property;
};

const generateOrbitPoints = (altitude: number, inclination: number, numPoints: number = 180): Cartesian3[] => {
  const inclinationRad = (inclination * Math.PI) / 180;
  const points: Cartesian3[] = [];
  for (let i = 0; i <= numPoints; i++) {
    const angle = (i / numPoints) * 2 * Math.PI;
    const x = Math.cos(angle);
    const y = Math.sin(angle) * Math.cos(inclinationRad);
    const z = Math.sin(angle) * Math.sin(inclinationRad);
    const lon = Math.atan2(y, x) * (180 / Math.PI);
    const lat = Math.asin(z) * (180 / Math.PI);
    points.push(Cartesian3.fromDegrees(lon, lat, altitude * 1000));
  }
  return points;
};

/** Near-equatorial / GEO-style ring: one revolution of longitude at the subsatellite lat. */
const generateEquatorialLikeRing = (
  altitudeKm: number,
  subsatLatDeg: number,
  subsatLngDeg: number,
  numPoints: number = 180
): Cartesian3[] => {
  const points: Cartesian3[] = [];
  for (let k = 0; k <= numPoints; k++) {
    let lonDeg = subsatLngDeg + (k / numPoints) * 360;
    while (lonDeg > 180) lonDeg -= 360;
    while (lonDeg < -180) lonDeg += 360;
    points.push(Cartesian3.fromDegrees(lonDeg, subsatLatDeg, altitudeKm * 1000));
  }
  return points;
};

/**
 * Same geometry as unphased `generateOrbitPoints`, rotated so the ring passes through the subsatellite point.
 * Uses geodetic subsat so the polyline matches billboard positions driven by `Lat`/`Lng`/`Alt` in live mode.
 */
const generateOrbitPointsPhased = (
  altitudeKm: number,
  inclinationDeg: number,
  subsatLatDeg: number,
  subsatLngDeg: number,
  orbitType: SatelliteData["orbitType"],
  numPoints: number = 180
): Cartesian3[] => {
  if (orbitType === "GEO" || inclinationDeg < 0.5) {
    return generateEquatorialLikeRing(altitudeKm, subsatLatDeg, subsatLngDeg, numPoints);
  }
  const i = (inclinationDeg * Math.PI) / 180;
  const lat0 = (subsatLatDeg * Math.PI) / 180;
  const lon0 = (subsatLngDeg * Math.PI) / 180;
  const sinI = Math.sin(i);
  let sa = sinI !== 0 ? Math.sin(lat0) / sinI : 0;
  sa = Math.max(-1, Math.min(1, sa));
  const theta = Math.asin(sa);
  const lonForTheta = (th: number) => Math.atan2(Math.sin(th) * Math.cos(i), Math.cos(th));
  let phase = lon0 - lonForTheta(theta);
  while (phase > Math.PI) phase -= 2 * Math.PI;
  while (phase <= -Math.PI) phase += 2 * Math.PI;

  const points: Cartesian3[] = [];
  for (let k = 0; k <= numPoints; k++) {
    const ang = (k / numPoints) * 2 * Math.PI;
    const x = Math.cos(ang);
    const y = Math.sin(ang) * Math.cos(i);
    const z = Math.sin(ang) * Math.sin(i);
    const lat = Math.asin(Math.max(-1, Math.min(1, z)));
    const lon = Math.atan2(y, x) + phase;
    let lonDeg = (lon * 180) / Math.PI;
    const latDeg = (lat * 180) / Math.PI;
    while (lonDeg > 180) lonDeg -= 360;
    while (lonDeg < -180) lonDeg += 360;
    points.push(Cartesian3.fromDegrees(lonDeg, latDeg, altitudeKm * 1000));
  }
  return points;
};

// Generate a satellite-shaped icon as a data URI canvas
const createSatelliteIcon = (color: Color, size: number = 32): string => {
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  const cx = size / 2;
  const cy = size / 2;
  const cssColor = color.toCssColorString();

  // Solar panels (two rectangles on each side)
  ctx.fillStyle = cssColor;
  ctx.globalAlpha = 0.8;
  ctx.fillRect(1, cy - 3, size / 3 - 2, 6);
  ctx.fillRect(size - size / 3 + 1, cy - 3, size / 3 - 2, 6);

  // Panel lines
  ctx.strokeStyle = "rgba(255,255,255,0.5)";
  ctx.lineWidth = 0.5;
  const panelW = size / 3 - 2;
  for (let i = 1; i < 3; i++) {
    const lx = 1 + (panelW / 3) * i;
    ctx.beginPath(); ctx.moveTo(lx, cy - 3); ctx.lineTo(lx, cy + 3); ctx.stroke();
    const rx = size - size / 3 + 1 + (panelW / 3) * i;
    ctx.beginPath(); ctx.moveTo(rx, cy - 3); ctx.lineTo(rx, cy + 3); ctx.stroke();
  }

  // Body (center square with rounded corners)
  ctx.globalAlpha = 1;
  ctx.fillStyle = cssColor;
  const bodySize = size * 0.3;
  const bodyX = cx - bodySize / 2;
  const bodyY = cy - bodySize / 2;
  ctx.beginPath();
  ctx.roundRect(bodyX, bodyY, bodySize, bodySize, 2);
  ctx.fill();

  // Body highlight
  ctx.fillStyle = "rgba(255,255,255,0.3)";
  ctx.fillRect(bodyX + 1, bodyY + 1, bodySize - 2, bodySize / 2 - 1);

  // Antenna
  ctx.strokeStyle = cssColor;
  ctx.lineWidth = 1.5;
  ctx.beginPath();
  ctx.moveTo(cx, bodyY);
  ctx.lineTo(cx, bodyY - 5);
  ctx.stroke();
  ctx.beginPath();
  ctx.arc(cx, bodyY - 6, 2, 0, Math.PI * 2);
  ctx.fillStyle = cssColor;
  ctx.fill();

  // White outline glow
  ctx.shadowColor = "white";
  ctx.shadowBlur = 3;
  ctx.strokeStyle = "rgba(255,255,255,0.6)";
  ctx.lineWidth = 0.5;
  ctx.beginPath();
  ctx.roundRect(bodyX, bodyY, bodySize, bodySize, 2);
  ctx.stroke();
  ctx.shadowBlur = 0;

  return canvas.toDataURL();
};

// Generate a ground station icon as a data URI canvas
const createGroundStationIcon = (size: number = 32): string => {
  const canvas = document.createElement("canvas");
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext("2d")!;
  const cx = size / 2;

  // Dish base
  ctx.fillStyle = "rgba(0, 255, 255, 0.9)";
  ctx.beginPath();
  ctx.moveTo(cx - 6, size - 4);
  ctx.lineTo(cx + 6, size - 4);
  ctx.lineTo(cx + 3, size - 8);
  ctx.lineTo(cx - 3, size - 8);
  ctx.closePath();
  ctx.fill();

  // Support pole
  ctx.strokeStyle = "rgba(0, 255, 255, 0.9)";
  ctx.lineWidth = 2;
  ctx.beginPath();
  ctx.moveTo(cx, size - 8);
  ctx.lineTo(cx, size / 2 + 2);
  ctx.stroke();

  // Dish (parabolic arc)
  ctx.lineWidth = 2.5;
  ctx.beginPath();
  ctx.arc(cx, size / 2 - 2, 10, Math.PI * 0.15, Math.PI * 0.85, false);
  ctx.stroke();
  ctx.fillStyle = "rgba(0, 255, 255, 0.25)";
  ctx.beginPath();
  ctx.arc(cx, size / 2 - 2, 10, Math.PI * 0.15, Math.PI * 0.85, false);
  ctx.lineTo(cx, size / 2 + 2);
  ctx.closePath();
  ctx.fill();

  // Signal waves
  ctx.strokeStyle = "rgba(0, 255, 255, 0.5)";
  ctx.lineWidth = 1;
  for (let i = 1; i <= 3; i++) {
    ctx.beginPath();
    ctx.arc(cx, 4, i * 3, Math.PI * 0.3, Math.PI * 0.7, false);
    ctx.stroke();
  }

  // White glow
  ctx.shadowColor = "cyan";
  ctx.shadowBlur = 4;
  ctx.fillStyle = "rgba(0, 255, 255, 0.8)";
  ctx.beginPath();
  ctx.arc(cx, size / 2 - 2, 2, 0, Math.PI * 2);
  ctx.fill();
  ctx.shadowBlur = 0;

  return canvas.toDataURL();
};

const defaultSatellites: SatelliteData[] = [
  { id: "leo1", name: "Sentinel-1A", orbitType: "LEO", altitude: 693, inclination: 98.18, color: Color.LIME, startAngle: 0 },
  { id: "leo2", name: "Sentinel-2A", orbitType: "LEO", altitude: 786, inclination: 98.62, color: Color.LIME, startAngle: Math.PI / 2 },
  { id: "leo3", name: "Sentinel-3A", orbitType: "LEO", altitude: 814, inclination: 98.65, color: Color.LIME, startAngle: Math.PI },
  { id: "leo4", name: "CryoSat-2", orbitType: "LEO", altitude: 717, inclination: 92, color: Color.LIME, startAngle: Math.PI * 1.5 },
  { id: "leo5", name: "SMOS", orbitType: "LEO", altitude: 758, inclination: 98.44, color: Color.LIME, startAngle: Math.PI * 0.25 },
  { id: "leo6", name: "Aeolus", orbitType: "LEO", altitude: 320, inclination: 97, color: Color.LIME, startAngle: Math.PI * 0.75 },
  { id: "leo7", name: "GOCE", orbitType: "LEO", altitude: 260, inclination: 96.5, color: Color.LIME, startAngle: Math.PI * 1.25 },
  { id: "leo8", name: "Swarm-A", orbitType: "LEO", altitude: 462, inclination: 87.35, color: Color.LIME, startAngle: Math.PI * 1.75 },
  { id: "meo1", name: "Galileo-1", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: 0 },
  { id: "meo2", name: "Galileo-2", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI / 2 },
  { id: "meo3", name: "Galileo-3", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI },
  { id: "meo4", name: "Galileo-4", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI * 1.5 },
  { id: "geo1", name: "Meteosat-11", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: 0 },
  { id: "geo2", name: "Meteosat-10", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: Math.PI * 0.66 },
  { id: "geo3", name: "MSG-4", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: Math.PI * 1.33 },
];

const defaultGroundStations: GroundStationData[] = [
  { id: "gs1", name: "Kiruna", lat: 67.857, lon: 20.964 },
  { id: "gs2", name: "Redu", lat: 50.002, lon: 5.146 },
  { id: "gs3", name: "Cebreros", lat: 40.453, lon: -4.368 },
  { id: "gs4", name: "Maspalomas", lat: 27.763, lon: -15.633 },
  { id: "gs5", name: "Kourou", lat: 5.252, lon: -52.786 },
  { id: "gs6", name: "New Norcia", lat: -31.048, lon: 116.192 },
];

const CesiumScene = ({
  orbitLayerVisibility,
  showGroundStations,
  showOrbits,
  showDataTransfer = false,
  showGroundLinks = false,
  showTrails = false,
  simulationSpeed = 1,
  isPaused = false,
  maxLinkDistance = 8000,
  satellites: satellitesProp,
  groundStationsList: groundStationsProp,
  liveMode = false,
  liveTracksRef,
  onSatelliteClick,
  onGroundStationClick,
}: CesiumSceneProps) => {
  const hideSimulationTimeControls = liveMode;
  const containerRef = useRef<HTMLDivElement>(null);
  const viewerRef = useRef<Viewer | null>(null);
  const connectionsRef = useRef<{ satLinks: Record<string, string[]>; gsLinks: Record<string, string[]>; stationToSat: Record<string, string> }>({ satLinks: {}, gsLinks: {}, stationToSat: {} });
  const [isInitialized, setIsInitialized] = useState(false);
  const orbitLayerVisRef = useRef(orbitLayerVisibility);
  orbitLayerVisRef.current = orbitLayerVisibility;

  const satellites = satellitesProp || defaultSatellites;
  const groundStationsList = groundStationsProp || defaultGroundStations;

  // Initialize Cesium viewer
  useEffect(() => {
    if (!containerRef.current || viewerRef.current) return;

    const initViewer = async () => {
      try {
        const viewer = new Viewer(containerRef.current!, {
          animation: true,
          timeline: true,
          homeButton: true,
          sceneModePicker: true,
          baseLayerPicker: false,
          navigationHelpButton: false,
          geocoder: false,
          fullscreenButton: false,
          vrButton: false,
          selectionIndicator: true,
          infoBox: false,
          shouldAnimate: true,
        });

        viewer.scene.renderError.addEventListener((_scene, err) => {
          console.error("Cesium render error:", err);
          try {
            const anyErr = err as unknown as { message?: string; stack?: string };
            console.error("message:", anyErr?.message, "stack:", anyErr?.stack);
          } catch { /* ignore */ }
        });

        try {
          const imageryProvider = await IonImageryProvider.fromAssetId(2);
          viewer.imageryLayers.addImageryProvider(imageryProvider);
        } catch (e) {
          console.log("Using default imagery", e);
        }

        viewer.camera.setView({
          destination: Cartesian3.fromDegrees(10, 30, 35000000),
        });

        const startTime = JulianDate.now();
        const stopTime = JulianDate.addSeconds(startTime, 86400, new JulianDate());

        viewer.clock.startTime = startTime.clone();
        viewer.clock.stopTime = stopTime.clone();
        viewer.clock.currentTime = startTime.clone();
        viewer.clock.clockRange = ClockRange.LOOP_STOP;
        viewer.clock.clockStep = ClockStep.SYSTEM_CLOCK_MULTIPLIER;
        viewer.clock.multiplier = 60;
        viewer.clock.shouldAnimate = true;

        viewer.timeline.zoomTo(startTime, stopTime);

        viewerRef.current = viewer;
        setIsInitialized(true);
      } catch (error) {
        console.error("Error initializing Cesium viewer:", error);
      }
    };

    initViewer();

    return () => {
      if (viewerRef.current && !viewerRef.current.isDestroyed()) {
        viewerRef.current.destroy();
        viewerRef.current = null;
      }
    };
  }, []);

  useEffect(() => {
    const viewer = viewerRef.current;
    if (!viewer || viewer.isDestroyed() || !isInitialized) return;
    const hide = hideSimulationTimeControls;
    const animEl = viewer.animation?.container as HTMLElement | undefined;
    const timeEl = viewer.timeline?.container as HTMLElement | undefined;
    if (animEl) animEl.style.display = hide ? "none" : "";
    if (timeEl) timeEl.style.display = hide ? "none" : "";
  }, [hideSimulationTimeControls, isInitialized]);

  // Handle entity clicks (satellites and ground stations)
  useEffect(() => {
    if (!viewerRef.current || !isInitialized) return;
    const viewer = viewerRef.current;

    const handler = new ScreenSpaceEventHandler(viewer.scene.canvas);
    handler.setInputAction((click: ScreenSpaceEventHandler.PositionedEvent) => {
      const pickedObjects = viewer.scene.drillPick(click.position, 10);
      for (const picked of pickedObjects) {
        if (!defined(picked) || !picked.id || !picked.id.id) continue;
        const entityId = picked.id.id as string;

        // Skip non-interactive entities
        if (entityId.startsWith("orbit-") || entityId.startsWith("isl-") || entityId.startsWith("gsl-") || entityId.startsWith("link-") || entityId.startsWith("pkt-")) continue;

        // Ground station click
        const gs = groundStationsList.find((s) => s.id === entityId);
        if (gs && onGroundStationClick) {
          viewer.selectedEntity = picked.id;
          const conn = connectionsRef.current;
          onGroundStationClick({
            id: gs.id,
            name: gs.name,
            lat: gs.lat,
            lon: gs.lon,
            connectedSatellite: conn.stationToSat[gs.id],
          });
          return;
        }

        // Satellite click
        const sat = satellites.find((s) => s.id === entityId);
        if (sat && onSatelliteClick) {
          viewer.selectedEntity = picked.id;
          const colorMap: Record<string, string> = { LEO: "#4ade80", MEO: "#facc15", GEO: "#f97316" };
          const conn = connectionsRef.current;
          const connectedSatNames = (conn.satLinks[sat.id] || []).map(
            (id) => satellites.find((s) => s.id === id)?.name || id
          );
          const connectedStationNames = conn.gsLinks[sat.id] || [];
          onSatelliteClick({
            id: sat.id,
            name: sat.name,
            orbitType: sat.orbitType,
            altitude: sat.altitude,
            inclination: sat.inclination,
            color: colorMap[sat.orbitType] || "#4ade80",
            connectedSatellites: connectedSatNames,
            connectedStations: connectedStationNames,
          });
          return;
        }
      }
    }, ScreenSpaceEventType.LEFT_CLICK);

    return () => handler.destroy();
  }, [isInitialized, onSatelliteClick, onGroundStationClick, satellites, groundStationsList]);

  // Update simulation speed and pause state
  useEffect(() => {
    if (!viewerRef.current || !isInitialized) return;
    viewerRef.current.clock.multiplier = simulationSpeed;
    viewerRef.current.clock.shouldAnimate = !isPaused;
  }, [simulationSpeed, isPaused, isInitialized]);

  // Update entities based on visibility toggles
  useEffect(() => {
    if (!viewerRef.current || !isInitialized) return;

    const viewer = viewerRef.current;
    const startTime = viewer.clock.startTime;

    viewer.entities.removeAll();

    const isFiniteLatLon = (lat: number, lon: number, alt: number) =>
      Number.isFinite(lat) && Number.isFinite(lon) && Number.isFinite(alt);

    // Add ground stations
    if (showGroundStations) {
      groundStationsList.forEach((station) => {
        const stationPosition =
          liveMode && liveTracksRef
            ? new CallbackProperty((_time, result) => {
                const t = liveTracksRef.current[station.id];
                const lng = t ? t.Lng : station.lon;
                const lat = t ? t.Lat : station.lat;
                const altKm = t ? (t.Alt ?? 0) : 0;
                if (!isFiniteLatLon(lat, lng, altKm)) {
                  return Cartesian3.fromDegrees(station.lon, station.lat, 0, Ellipsoid.WGS84, result);
                }
                return Cartesian3.fromDegrees(lng, lat, altKm * 1000, Ellipsoid.WGS84, result);
              }, false)
            : Cartesian3.fromDegrees(station.lon, station.lat, 0);

        viewer.entities.add({
          id: station.id,
          name: station.name,
          position: stationPosition,
          billboard: {
            image: createGroundStationIcon(36),
            width: 28,
            height: 28,
            heightReference: 1,
          },
          label: {
            text: station.name,
            font: "12px sans-serif",
            fillColor: Color.WHITE,
            outlineColor: Color.BLACK,
            outlineWidth: 2,
            style: 2,
            pixelOffset: new Cartesian2(0, -22),
            heightReference: 1,
            showBackground: true,
            backgroundColor: Color.BLACK.withAlpha(0.6),
            scale: 0.85,
          },
          description: `<div style="padding: 8px;"><h3>${station.name}</h3><p>ESA Ground Station</p><p>Lat: ${station.lat.toFixed(3)}°</p><p>Lon: ${station.lon.toFixed(3)}°</p></div>`,
        });
      });
    }

    const makeSatelliteShow = (sat: SatelliteData) =>
      new CallbackProperty(() => {
        if (orbitLayerVisRef.current[sat.orbitType] === false) return false;
        if (liveMode && liveTracksRef && !liveTracksRef.current[sat.id]) return false;
        return true;
      }, false);

    const makeOrbitShow = (sat: SatelliteData) =>
      new CallbackProperty(() => {
        if (orbitLayerVisRef.current[sat.orbitType] === false) return false;
        if (liveMode && liveTracksRef && !liveTracksRef.current[sat.id]) return false;
        return true;
      }, false);

    satellites.forEach((sat) => {
      if (showOrbits) {
        let positions: Cartesian3[] | CallbackProperty;
        if (liveMode && liveTracksRef) {
          positions = new CallbackProperty(() => {
            const t = liveTracksRef.current[sat.id];
            if (!t || !isFiniteLatLon(t.Lat, t.Lng, t.Alt)) return undefined;
            const altKm = t.Alt > 0 ? t.Alt : sat.altitude;
            const incEff = Math.max(sat.inclination, Math.abs(t.Lat) + 0.5, 0.5);
            return generateOrbitPointsPhased(altKm, incEff, t.Lat, t.Lng, sat.orbitType);
          }, false);
        } else {
          positions = generateOrbitPoints(sat.altitude, sat.inclination);
        }
        viewer.entities.add({
          id: `orbit-${sat.id}`,
          show: makeOrbitShow(sat),
          polyline: {
            positions: positions,
            width: 1.5,
            material: new PolylineGlowMaterialProperty({
              glowPower: 0.2,
              color: sat.color.withAlpha(0.4),
            }),
          },
        });
      }

      let position: SampledPositionProperty | CallbackProperty;
      let billRotation: number | CallbackProperty;

      if (liveMode && liveTracksRef) {
        position = new CallbackProperty((_time, result) => {
          const t = liveTracksRef.current[sat.id];
          if (!t || !isFiniteLatLon(t.Lat, t.Lng, t.Alt)) return undefined;
          return Cartesian3.fromDegrees(t.Lng, t.Lat, t.Alt * 1000, Ellipsoid.WGS84, result);
        }, false);
        billRotation = 0;
      } else {
        const orbitPosition = createOrbitPath(sat.altitude, sat.inclination, sat.startAngle, startTime, 86400);
        position = orbitPosition;
        const velocityVector = new VelocityVectorProperty(orbitPosition, false);
        billRotation = new CallbackProperty((time) => {
          const pos = orbitPosition.getValue(time);
          const vel = velocityVector.getValue(time);
          if (!pos || !vel) return 0;

          const transform = Transforms.eastNorthUpToFixedFrame(pos);
          const inverseTransform = Matrix4.inverse(transform, new Matrix4());
          const localVel = Matrix4.multiplyByPointAsVector(inverseTransform, vel, new Cartesian3());

          return -Math.atan2(localVel.x, localVel.y);
        }, false);
      }

      viewer.entities.add({
        id: sat.id,
        name: sat.name,
        show: makeSatelliteShow(sat),
        position: position,
        billboard: {
          image: createSatelliteIcon(sat.color, sat.orbitType === "GEO" ? 40 : sat.orbitType === "MEO" ? 36 : 32),
          width: sat.orbitType === "GEO" ? 28 : sat.orbitType === "MEO" ? 24 : 20,
          height: sat.orbitType === "GEO" ? 28 : sat.orbitType === "MEO" ? 24 : 20,
          rotation: billRotation,
        },
        label: {
          text: sat.name,
          font: "11px sans-serif",
          fillColor: sat.color,
          outlineColor: Color.BLACK,
          outlineWidth: 2,
          style: 2,
          pixelOffset: new Cartesian2(0, -15),
          showBackground: true,
          backgroundColor: Color.BLACK.withAlpha(0.6),
          scale: 0.8,
        },
        path:
          showTrails && !liveMode
            ? {
                width: 2,
                material: sat.color.withAlpha(0.6),
                leadTime: sat.orbitType === "LEO" ? 900 : sat.orbitType === "MEO" ? 3600 : 7200,
                trailTime: sat.orbitType === "LEO" ? 900 : sat.orbitType === "MEO" ? 3600 : 7200,
              }
            : undefined,
        description: `
          <div style="padding: 12px; font-family: sans-serif;">
            <h3 style="margin: 0 0 8px 0; color: ${sat.color.toCssColorString()};">${sat.name}</h3>
            <p style="margin: 4px 0;"><strong>Orbit Type:</strong> ${sat.orbitType}</p>
            <p style="margin: 4px 0;"><strong>Altitude:</strong> ${sat.altitude.toLocaleString()} km</p>
            <p style="margin: 4px 0;"><strong>Inclination:</strong> ${sat.inclination}°</p>
            <p style="margin: 4px 0;"><strong>Period:</strong> ${Math.round(2 * Math.PI * Math.sqrt(Math.pow(6371 + sat.altitude, 3) / 398600.4418) / 60)} min</p>
          </div>
        `,
      });
    });
  }, [
    isInitialized,
    showGroundStations,
    showOrbits,
    showTrails,
    satellites,
    groundStationsList,
    liveMode,
    liveTracksRef,
  ]);

  /** Replace show properties when layer toggles change — fresh Property instances so Cesium applies visibility. */
  useEffect(() => {
    if (!viewerRef.current || !isInitialized) return;
    const viewer = viewerRef.current;
    for (const sat of satellites) {
      const se = viewer.entities.getById(sat.id);
      const oe = viewer.entities.getById(`orbit-${sat.id}`);
      if (se) {
        se.show = new CallbackProperty(() => {
          if (orbitLayerVisRef.current[sat.orbitType] === false) return false;
          if (liveMode && liveTracksRef && !liveTracksRef.current[sat.id]) return false;
          return true;
        }, false);
      }
      if (oe) {
        oe.show = new CallbackProperty(() => {
          if (orbitLayerVisRef.current[sat.orbitType] === false) return false;
          if (liveMode && liveTracksRef && !liveTracksRef.current[sat.id]) return false;
          return true;
        }, false);
      }
    }
  }, [orbitLayerVisibility, isInitialized, satellites, liveMode, liveTracksRef, showOrbits]);

  // Dynamic data transfer & ground links - computed each tick
  useEffect(() => {
    if (!viewerRef.current || !isInitialized) return;
    if (!showDataTransfer && !showGroundLinks) return;

    const viewer = viewerRef.current;
    const EARTH_RADIUS = 6371000;
    const MIN_ELEVATION_DEG = 5;
    const MAX_INTER_SAT_DISTANCE_KM = maxLinkDistance;
    const linkEntities: Entity[] = [];

    let lastUpdateTime = 0;
    const UPDATE_INTERVAL_MS = 500;

    const getDistance = (pos1: Cartesian3, pos2: Cartesian3): number => {
      const dx = pos1.x - pos2.x;
      const dy = pos1.y - pos2.y;
      const dz = pos1.z - pos2.z;
      return Math.sqrt(dx * dx + dy * dy + dz * dz) / 1000; // km
    };

    const onTick = () => {
      const now = Date.now();
      if (now - lastUpdateTime < UPDATE_INTERVAL_MS) return;
      lastUpdateTime = now;

      // Remove previous link entities
      linkEntities.forEach((e) => {
        if (viewer.entities.contains(e)) viewer.entities.remove(e);
      });
      linkEntities.length = 0;

      // Track connections for popup info
      const satLinks: Record<string, string[]> = {};
      const gsLinks: Record<string, string[]> = {};
      const stationToSat: Record<string, string> = {};

      const currentTime = viewer.clock.currentTime;

      // Satellites whose orbit layer is on (and live track present when streaming)
      const satPositions: { sat: SatelliteData; position: Cartesian3 }[] = [];
      satellites.forEach((sat) => {
        const satEntity = viewer.entities.getById(sat.id);
        if (!satEntity || !satEntity.position) return;
        const pos = satEntity.position.getValue(currentTime);
        if (!pos) return;
        if (orbitLayerVisRef.current[sat.orbitType] === false) return;
        if (liveMode && !liveTracksRef?.current[sat.id]) return;
        satPositions.push({ sat, position: pos });
      });

      // Inter-satellite links: connect each satellite to its nearest neighbor within range
      // Each satellite can only have ONE inter-satellite link
      if (showDataTransfer) {
        const linkedSats = new Set<string>(); // track satellites already linked
        
        // Build candidate pairs sorted by distance
        const pairs: { a: typeof satPositions[0]; b: typeof satPositions[0]; dist: number }[] = [];
        for (let i = 0; i < satPositions.length; i++) {
          for (let j = i + 1; j < satPositions.length; j++) {
            const dist = getDistance(satPositions[i].position, satPositions[j].position);
            if (dist <= MAX_INTER_SAT_DISTANCE_KM) {
              pairs.push({ a: satPositions[i], b: satPositions[j], dist });
            }
          }
        }
        pairs.sort((x, y) => x.dist - y.dist);

        pairs.forEach(({ a, b, dist }) => {
          if (linkedSats.has(a.sat.id) || linkedSats.has(b.sat.id)) return;
          linkedSats.add(a.sat.id);
          linkedSats.add(b.sat.id);

          satLinks[a.sat.id] = [b.sat.id];
          satLinks[b.sat.id] = [a.sat.id];

          const entity = viewer.entities.add({
            id: `isl-${a.sat.id}-${b.sat.id}-${now}`,
            polyline: {
              positions: [a.position, b.position],
              width: 3,
              material: new PolylineDashMaterialProperty({
                color: Color.fromCssColorString("#ff44ff").withAlpha(0.8),
                gapColor: Color.TRANSPARENT,
                dashLength: 24,
                dashPattern: 255,
              }),
            },
          });
          linkEntities.push(entity);

          // Data packet traveling along the ISL
          const islPacketA = a.position.clone();
          const islPacketB = b.position.clone();
          const packetEntity = viewer.entities.add({
            id: `pkt-isl-${a.sat.id}-${b.sat.id}-${now}`,
            position: new CallbackProperty(() => {
              const t = (Date.now() % 2000) / 2000; // 2-second cycle
              return new Cartesian3(
                islPacketA.x + (islPacketB.x - islPacketA.x) * t,
                islPacketA.y + (islPacketB.y - islPacketA.y) * t,
                islPacketA.z + (islPacketB.z - islPacketA.z) * t,
              );
            }, false),
            point: {
              pixelSize: 5,
              color: Color.fromCssColorString("#ff88ff"),
              outlineColor: Color.fromCssColorString("#ff44ff"),
              outlineWidth: 2,
            },
          });
          linkEntities.push(packetEntity);

          // Second packet going the other direction (offset by half cycle)
          const packetEntity2 = viewer.entities.add({
            id: `pkt-isl2-${a.sat.id}-${b.sat.id}-${now}`,
            position: new CallbackProperty(() => {
              const t = ((Date.now() + 1000) % 2000) / 2000;
              return new Cartesian3(
                islPacketB.x + (islPacketA.x - islPacketB.x) * t,
                islPacketB.y + (islPacketA.y - islPacketB.y) * t,
                islPacketB.z + (islPacketA.z - islPacketB.z) * t,
              );
            }, false),
            point: {
              pixelSize: 4,
              color: Color.fromCssColorString("#ff88ff").withAlpha(0.7),
              outlineColor: Color.fromCssColorString("#ff44ff").withAlpha(0.5),
              outlineWidth: 1,
            },
          });
          linkEntities.push(packetEntity2);
        });
      }

      // Ground station links: each satellite can connect to at most ONE ground station
      if (showGroundLinks && showGroundStations) {
        const gsLinkedSats = new Set<string>(); // satellites already linked to a ground station

        groundStationsList.forEach((gs) => {
          const gsLat = (gs.lat * Math.PI) / 180;
          const gsLon = (gs.lon * Math.PI) / 180;
          const gsPosition = Cartesian3.fromDegrees(gs.lon, gs.lat, 0);

          let bestSat: { sat: SatelliteData; position: Cartesian3; elevation: number } | null = null;

          satPositions.forEach(({ sat, position: satPosition }) => {
            if (gsLinkedSats.has(sat.id)) return; // already connected to another station

            const satCartographic = Cartographic.fromCartesian(satPosition);
            const satLat = satCartographic.latitude;
            const satLon = satCartographic.longitude;
            const satAlt = satCartographic.height;

            const dLon = satLon - gsLon;
            const cosCA = Math.sin(gsLat) * Math.sin(satLat) +
              Math.cos(gsLat) * Math.cos(satLat) * Math.cos(dLon);
            const centralAngle = Math.acos(Math.min(1, Math.max(-1, cosCA)));

            const R = EARTH_RADIUS;
            const r = R + satAlt;
            const elevationRad = Math.atan2(
              r * Math.cos(centralAngle) - R,
              r * Math.sin(centralAngle)
            );
            const elevationDeg = (elevationRad * 180) / Math.PI;

            if (elevationDeg >= MIN_ELEVATION_DEG) {
              if (!bestSat || elevationDeg > bestSat.elevation) {
                bestSat = { sat, position: satPosition, elevation: elevationDeg };
              }
            }
          });

          if (bestSat) {
            gsLinkedSats.add(bestSat.sat.id);

            let linkColor = Color.CYAN.withAlpha(0.4);
            if (bestSat.sat.orbitType === "MEO") linkColor = Color.YELLOW.withAlpha(0.35);
            if (bestSat.sat.orbitType === "GEO") linkColor = Color.ORANGERED.withAlpha(0.35);

            gsLinks[bestSat.sat.id] = [gs.name];
            stationToSat[gs.id] = bestSat.sat.name;

            const entity = viewer.entities.add({
              id: `gsl-${gs.id}-${bestSat.sat.id}-${now}`,
              polyline: {
                positions: [gsPosition, bestSat.position],
                width: 1.5,
                material: new PolylineDashMaterialProperty({
                  color: linkColor,
                  gapColor: Color.TRANSPARENT,
                  dashLength: 12,
                  dashPattern: 255,
                }),
              },
            });
            linkEntities.push(entity);

            // Data packet traveling from ground station up to satellite
            const gsPos = gsPosition.clone();
            const satPos = bestSat.position.clone();
            const gslPacket = viewer.entities.add({
              id: `pkt-gsl-${gs.id}-${bestSat.sat.id}-${now}`,
              position: new CallbackProperty(() => {
                const t = (Date.now() % 3000) / 3000; // 3-second cycle
                return new Cartesian3(
                  gsPos.x + (satPos.x - gsPos.x) * t,
                  gsPos.y + (satPos.y - gsPos.y) * t,
                  gsPos.z + (satPos.z - gsPos.z) * t,
                );
              }, false),
              point: {
                pixelSize: 4,
                color: Color.CYAN,
                outlineColor: Color.WHITE.withAlpha(0.5),
                outlineWidth: 1,
              },
            });
            linkEntities.push(gslPacket);

            // Downlink packet (satellite to ground)
            const gslPacket2 = viewer.entities.add({
              id: `pkt-gsl2-${gs.id}-${bestSat.sat.id}-${now}`,
              position: new CallbackProperty(() => {
                const t = ((Date.now() + 1500) % 3000) / 3000;
                return new Cartesian3(
                  satPos.x + (gsPos.x - satPos.x) * t,
                  satPos.y + (gsPos.y - satPos.y) * t,
                  satPos.z + (gsPos.z - satPos.z) * t,
                );
              }, false),
              point: {
                pixelSize: 3,
                color: Color.CYAN.withAlpha(0.7),
                outlineWidth: 0,
              },
            });
            linkEntities.push(gslPacket2);
          }
        });

      }

      // Store connections for click handler
      connectionsRef.current = { satLinks, gsLinks, stationToSat };
    };

    viewer.clock.onTick.addEventListener(onTick);

    return () => {
      if (!viewer.isDestroyed()) {
        viewer.clock.onTick.removeEventListener(onTick);
        linkEntities.forEach((e) => {
          if (viewer.entities.contains(e)) viewer.entities.remove(e);
        });
      }
    };
  }, [isInitialized, showDataTransfer, showGroundLinks, showGroundStations, maxLinkDistance, satellites, groundStationsList, liveMode, liveTracksRef]);

  return (
    <div
      ref={containerRef}
      className="cesium-container"
      style={{ width: "100%", height: "100%", position: "absolute", inset: 0 }}
    />
  );
};

export default CesiumScene;
