import * as satellite from "satellite.js";

export interface TLEData {
  name: string;
  line1: string;
  line2: string;
}

export interface SatellitePosition {
  latitude: number;
  longitude: number;
  altitude: number; // km
  velocity: number; // km/s
}

export interface PassPrediction {
  startTime: Date;
  endTime: Date;
  maxElevation: number;
  azimuthStart: number;
  azimuthEnd: number;
}

// Sample TLE data for demonstration (real ESA/scientific satellites)
export const sampleTLEs: TLEData[] = [
  // ISS
  {
    name: "ISS (ZARYA)",
    line1: "1 25544U 98067A   24001.50000000  .00016717  00000-0  10270-3 0  9025",
    line2: "2 25544  51.6416 208.5481 0005684  35.4627 324.6793 15.49961890378034",
  },
  // Sentinel-1A
  {
    name: "SENTINEL-1A",
    line1: "1 39634U 14016A   24001.50000000  .00000044  00000-0  28396-4 0  9999",
    line2: "2 39634  98.1799 272.8452 0001234  77.9521 282.1799 14.59198520507556",
  },
  // Sentinel-2A
  {
    name: "SENTINEL-2A",
    line1: "1 40697U 15028A   24001.50000000  .00000035  00000-0  19231-4 0  9992",
    line2: "2 40697  98.5694 282.7892 0001122 100.1234 259.9876 14.30818456449012",
  },
  // Copernicus Sentinel-3A
  {
    name: "SENTINEL-3A",
    line1: "1 41335U 16011A   24001.50000000  .00000012  00000-0  15876-4 0  9996",
    line2: "2 41335  98.6502 123.4567 0001089  88.7654 271.3456 14.26749123390211",
  },
  // MetOp-C
  {
    name: "METOP-C",
    line1: "1 43689U 18087A   24001.50000000  .00000023  00000-0  20156-4 0  9993",
    line2: "2 43689  98.7012 345.6789 0002345  67.8901 292.2345 14.21552134278901",
  },
  // Aeolus
  {
    name: "AEOLUS",
    line1: "1 43600U 18066A   24001.50000000  .00000156  00000-0  31245-4 0  9997",
    line2: "2 43600  96.7123 234.5678 0007654  45.6789 314.4321 15.19876543210987",
  },
  // CryoSat-2
  {
    name: "CRYOSAT-2",
    line1: "1 36508U 10013A   24001.50000000  .00000019  00000-0  18765-4 0  9991",
    line2: "2 36508  92.0234  56.7890 0001234 123.4567 236.6543 14.52176890456789",
  },
  // GOCE (simulated - deorbited but for demo)
  {
    name: "SWARM-A",
    line1: "1 39452U 13067B   24001.50000000  .00001234  00000-0  45678-4 0  9998",
    line2: "2 39452  87.3456 178.9012 0004567  89.0123 271.0987 15.53210987654321",
  },
  // GPS BIIR-2
  {
    name: "GPS BIIR-2",
    line1: "1 24876U 97035A   24001.50000000 -.00000012  00000-0  00000+0 0  9995",
    line2: "2 24876  55.7234  67.8901 0045678 234.5678 125.4321 2.00562987654321",
  },
  // GALILEO-1
  {
    name: "GALILEO-1",
    line1: "1 37846U 11060A   24001.50000000 -.00000015  00000-0  00000+0 0  9994",
    line2: "2 37846  56.0456 123.4567 0003456 278.9012  81.0987 1.70234567890123",
  },
  // INTELSAT 10-02 (GEO)
  {
    name: "INTELSAT 10-02",
    line1: "1 28358U 04022A   24001.50000000  .00000012  00000-0  00000+0 0  9996",
    line2: "2 28358   0.0234 234.5678 0002345 123.4567 236.5432 1.00272345678901",
  },
  // EUTELSAT 7A (GEO)
  {
    name: "EUTELSAT 7A",
    line1: "1 29273U 06032A   24001.50000000  .00000008  00000-0  00000+0 0  9992",
    line2: "2 29273   0.0456  45.6789 0003456  89.0123 270.9876 1.00271234567890",
  },
];

// Parse TLE and get satellite record
export function parseTLE(tle: TLEData): satellite.SatRec | null {
  try {
    const satrec = satellite.twoline2satrec(tle.line1, tle.line2);
    return satrec;
  } catch (error) {
    console.error(`Error parsing TLE for ${tle.name}:`, error);
    return null;
  }
}

// Get satellite position at a specific time
export function getSatellitePosition(
  satrec: satellite.SatRec,
  date: Date
): SatellitePosition | null {
  try {
    const positionAndVelocity = satellite.propagate(satrec, date);
    
    if (!positionAndVelocity.position || typeof positionAndVelocity.position === "boolean") {
      return null;
    }

    const positionEci = positionAndVelocity.position as satellite.EciVec3<number>;
    const velocityEci = positionAndVelocity.velocity as satellite.EciVec3<number>;

    const gmst = satellite.gstime(date);
    const positionGd = satellite.eciToGeodetic(positionEci, gmst);

    const longitude = satellite.degreesLong(positionGd.longitude);
    const latitude = satellite.degreesLat(positionGd.latitude);
    const altitude = positionGd.height;

    const velocity = Math.sqrt(
      velocityEci.x ** 2 + velocityEci.y ** 2 + velocityEci.z ** 2
    );

    return {
      latitude,
      longitude,
      altitude,
      velocity,
    };
  } catch (error) {
    return null;
  }
}

// Get ECI position for 3D rendering
export function getECIPosition(
  satrec: satellite.SatRec,
  date: Date
): { x: number; y: number; z: number } | null {
  try {
    const positionAndVelocity = satellite.propagate(satrec, date);
    
    if (!positionAndVelocity.position || typeof positionAndVelocity.position === "boolean") {
      return null;
    }

    const positionEci = positionAndVelocity.position as satellite.EciVec3<number>;
    
    // Scale to scene units (Earth radius = 2 in our scene)
    const scale = 2 / 6371; // Earth's radius in km
    
    return {
      x: positionEci.x * scale,
      y: positionEci.z * scale, // Swap Y and Z for Three.js coordinate system
      z: -positionEci.y * scale,
    };
  } catch (error) {
    return null;
  }
}

// Calculate satellite passes over a ground station
export function predictPasses(
  satrec: satellite.SatRec,
  groundStation: { lat: number; lng: number; alt?: number },
  startDate: Date,
  hoursAhead: number = 24
): PassPrediction[] {
  const passes: PassPrediction[] = [];
  const observerGd = {
    longitude: satellite.degreesToRadians(groundStation.lng),
    latitude: satellite.degreesToRadians(groundStation.lat),
    height: (groundStation.alt || 0) / 1000, // Convert to km
  };

  const endTime = new Date(startDate.getTime() + hoursAhead * 60 * 60 * 1000);
  const stepMs = 60 * 1000; // 1 minute steps
  
  let currentPass: {
    start: Date;
    maxEl: number;
    maxElTime: Date;
    azStart: number;
    azEnd: number;
  } | null = null;
  
  let prevElevation = -90;

  for (let time = startDate.getTime(); time < endTime.getTime(); time += stepMs) {
    const date = new Date(time);
    const positionAndVelocity = satellite.propagate(satrec, date);
    
    if (!positionAndVelocity.position || typeof positionAndVelocity.position === "boolean") {
      continue;
    }

    const positionEci = positionAndVelocity.position as satellite.EciVec3<number>;
    const gmst = satellite.gstime(date);
    const positionEcf = satellite.eciToEcf(positionEci, gmst);
    const lookAngles = satellite.ecfToLookAngles(observerGd, positionEcf);
    
    const elevation = satellite.radiansToDegrees(lookAngles.elevation);
    const azimuth = satellite.radiansToDegrees(lookAngles.azimuth);

    // Satellite is above horizon
    if (elevation > 0) {
      if (!currentPass) {
        // Start of pass
        currentPass = {
          start: date,
          maxEl: elevation,
          maxElTime: date,
          azStart: azimuth,
          azEnd: azimuth,
        };
      } else {
        // Update pass
        if (elevation > currentPass.maxEl) {
          currentPass.maxEl = elevation;
          currentPass.maxElTime = date;
        }
        currentPass.azEnd = azimuth;
      }
    } else if (currentPass && prevElevation > 0) {
      // End of pass
      passes.push({
        startTime: currentPass.start,
        endTime: date,
        maxElevation: currentPass.maxEl,
        azimuthStart: currentPass.azStart,
        azimuthEnd: currentPass.azEnd,
      });
      currentPass = null;
    }

    prevElevation = elevation;
  }

  return passes.slice(0, 10); // Return max 10 passes
}

// Get orbit type based on altitude
export function getOrbitType(altitude: number): "LEO" | "MEO" | "GEO" {
  if (altitude < 2000) return "LEO";
  if (altitude < 35000) return "MEO";
  return "GEO";
}

// Get color based on orbit type
export function getOrbitColor(orbitType: "LEO" | "MEO" | "GEO"): string {
  switch (orbitType) {
    case "LEO": return "#4ade80";
    case "MEO": return "#facc15";
    case "GEO": return "#f97316";
  }
}
