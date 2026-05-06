import { Color } from "cesium";

export interface SatelliteData {
  id: string;
  name: string;
  orbitType: "LEO" | "MEO" | "GEO";
  altitude: number;
  inclination: number;
  color: Color;
  startAngle: number;
}

export interface GroundStationData {
  id: string;
  name: string;
  lat: number;
  lon: number;
}

export interface ExperimentConfig {
  id: string;
  name: string;
  satellites: SatelliteData[];
  groundStations: GroundStationData[];
}

const allGroundStations: GroundStationData[] = [
  { id: "gs1", name: "Kiruna", lat: 67.857, lon: 20.964 },
  { id: "gs2", name: "Redu", lat: 50.002, lon: 5.146 },
  { id: "gs3", name: "Cebreros", lat: 40.453, lon: -4.368 },
  { id: "gs4", name: "Maspalomas", lat: 27.763, lon: -15.633 },
  { id: "gs5", name: "Kourou", lat: 5.252, lon: -52.786 },
  { id: "gs6", name: "New Norcia", lat: -31.048, lon: 116.192 },
];

/** LEO + MEO + GEO mix and varied phases — for default viz and EXP-DEMO. */
const fullDemoSatellites: SatelliteData[] = [
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

export const experimentConfigs: Record<string, ExperimentConfig> = {
  "EXP-001": {
    id: "EXP-001",
    name: "Satellite Network Live",
    satellites: [
      { id: "leo1", name: "Sentinel-1A", orbitType: "LEO", altitude: 693, inclination: 98.18, color: Color.LIME, startAngle: 0 },
      { id: "leo2", name: "Sentinel-2A", orbitType: "LEO", altitude: 786, inclination: 98.62, color: Color.LIME, startAngle: Math.PI / 2 },
      { id: "leo3", name: "Sentinel-3A", orbitType: "LEO", altitude: 814, inclination: 98.65, color: Color.LIME, startAngle: Math.PI },
      { id: "leo4", name: "CryoSat-2", orbitType: "LEO", altitude: 717, inclination: 92, color: Color.LIME, startAngle: Math.PI * 1.5 },
      { id: "leo5", name: "SMOS", orbitType: "LEO", altitude: 758, inclination: 98.44, color: Color.LIME, startAngle: Math.PI * 0.25 },
      { id: "leo6", name: "Aeolus", orbitType: "LEO", altitude: 320, inclination: 97, color: Color.LIME, startAngle: Math.PI * 0.75 },
      { id: "leo7", name: "GOCE", orbitType: "LEO", altitude: 260, inclination: 96.5, color: Color.LIME, startAngle: Math.PI * 1.25 },
      { id: "leo8", name: "Swarm-A", orbitType: "LEO", altitude: 462, inclination: 87.35, color: Color.LIME, startAngle: Math.PI * 1.75 },
      { id: "leo9", name: "Swarm-B", orbitType: "LEO", altitude: 510, inclination: 87.35, color: Color.LIME, startAngle: Math.PI * 0.5 },
      { id: "leo10", name: "Swarm-C", orbitType: "LEO", altitude: 462, inclination: 87.35, color: Color.LIME, startAngle: Math.PI * 1.1 },
      { id: "leo11", name: "Sentinel-5P", orbitType: "LEO", altitude: 824, inclination: 98.74, color: Color.LIME, startAngle: Math.PI * 0.6 },
      { id: "leo12", name: "Sentinel-6", orbitType: "LEO", altitude: 1336, inclination: 66, color: Color.LIME, startAngle: Math.PI * 1.6 },
    ],
    groundStations: [allGroundStations[0], allGroundStations[1], allGroundStations[3], allGroundStations[4]],
  },
  "EXP-002": {
    id: "EXP-002",
    name: "Disaster Response Network Test",
    satellites: [
      { id: "leo1", name: "Sentinel-1A", orbitType: "LEO", altitude: 693, inclination: 98.18, color: Color.LIME, startAngle: 0 },
      { id: "leo2", name: "Sentinel-2A", orbitType: "LEO", altitude: 786, inclination: 98.62, color: Color.LIME, startAngle: Math.PI / 2 },
      { id: "leo3", name: "Sentinel-3A", orbitType: "LEO", altitude: 814, inclination: 98.65, color: Color.LIME, startAngle: Math.PI },
      { id: "geo1", name: "Meteosat-11", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: 0 },
      { id: "geo2", name: "Meteosat-10", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: Math.PI * 0.66 },
      { id: "geo3", name: "MSG-4", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: Math.PI * 1.33 },
      { id: "meo1", name: "Galileo-1", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: 0 },
      { id: "meo2", name: "Galileo-2", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI / 2 },
    ],
    groundStations: allGroundStations,
  },
  "EXP-003": {
    id: "EXP-003",
    name: "MEO-GEO Hybrid Routing",
    satellites: [
      { id: "meo1", name: "Galileo-1", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: 0 },
      { id: "meo2", name: "Galileo-2", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI / 2 },
      { id: "meo3", name: "Galileo-3", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI },
      { id: "meo4", name: "Galileo-4", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI * 1.5 },
      { id: "meo5", name: "Galileo-5", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI * 0.33 },
      { id: "meo6", name: "Galileo-6", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI * 0.83 },
      { id: "meo7", name: "Galileo-7", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI * 1.33 },
      { id: "meo8", name: "Galileo-8", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: Math.PI * 1.83 },
      { id: "geo1", name: "Meteosat-11", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: 0 },
      { id: "geo2", name: "Meteosat-10", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: Math.PI * 0.66 },
      { id: "geo3", name: "MSG-4", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: Math.PI * 1.33 },
      { id: "geo4", name: "EDRS-A", orbitType: "GEO", altitude: 35786, inclination: 0.05, color: Color.RED, startAngle: Math.PI * 0.25 },
      { id: "geo5", name: "EDRS-C", orbitType: "GEO", altitude: 35786, inclination: 0.05, color: Color.RED, startAngle: Math.PI * 1.0 },
      { id: "leo1", name: "Sentinel-1A", orbitType: "LEO", altitude: 693, inclination: 98.18, color: Color.LIME, startAngle: 0 },
      { id: "leo2", name: "Sentinel-2A", orbitType: "LEO", altitude: 786, inclination: 98.62, color: Color.LIME, startAngle: Math.PI / 2 },
      { id: "leo3", name: "Sentinel-3A", orbitType: "LEO", altitude: 814, inclination: 98.65, color: Color.LIME, startAngle: Math.PI },
    ],
    groundStations: [allGroundStations[0], allGroundStations[1], allGroundStations[2], allGroundStations[3], allGroundStations[4], allGroundStations[5], 
      { id: "gs7", name: "Malindi", lat: -2.996, lon: 40.194 },
      { id: "gs8", name: "Santa Maria", lat: 36.997, lon: -25.136 },
    ],
  },
  "EXP-004": {
    id: "EXP-004",
    name: "Blockchain Provenance Trial",
    satellites: [
      { id: "leo1", name: "Sentinel-1A", orbitType: "LEO", altitude: 693, inclination: 98.18, color: Color.LIME, startAngle: 0 },
      { id: "leo2", name: "Sentinel-2A", orbitType: "LEO", altitude: 786, inclination: 98.62, color: Color.LIME, startAngle: Math.PI / 2 },
      { id: "leo3", name: "Sentinel-3A", orbitType: "LEO", altitude: 814, inclination: 98.65, color: Color.LIME, startAngle: Math.PI },
      { id: "meo1", name: "Galileo-1", orbitType: "MEO", altitude: 23222, inclination: 56, color: Color.YELLOW, startAngle: 0 },
      { id: "geo1", name: "Meteosat-11", orbitType: "GEO", altitude: 35786, inclination: 0.1, color: Color.RED, startAngle: 0 },
      { id: "geo2", name: "EDRS-A", orbitType: "GEO", altitude: 35786, inclination: 0.05, color: Color.RED, startAngle: Math.PI * 0.5 },
    ],
    groundStations: [allGroundStations[0], allGroundStations[2], allGroundStations[4]],
  },
  "EXP-DEMO": {
    id: "EXP-DEMO",
    name: "Interactive demo (full features)",
    satellites: fullDemoSatellites,
    groundStations: allGroundStations,
  },
};

// Default config used when no experiment ID is provided
export const defaultExperimentConfig: ExperimentConfig = {
  id: "default",
  name: "All Satellites",
  satellites: fullDemoSatellites,
  groundStations: allGroundStations,
};

export const getExperimentConfig = (experimentId?: string): ExperimentConfig => {
  if (!experimentId) return defaultExperimentConfig;
  return experimentConfigs[experimentId] || defaultExperimentConfig;
};
