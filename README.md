# ESA Space Operations — Satellite Constellation Viewer

Interactive 3D visualization platform for ESA satellite constellations, featuring real-time orbital tracking, inter-satellite link simulation, and ground station monitoring.

## Features

- **3D Globe Visualization** — CesiumJS-powered globe with real-time satellite positioning using SGP4/SDP4 orbital propagation (satellite.js)
- **Multi-Orbit Support** — View and filter LEO, MEO, and GEO satellite constellations with distinct color coding
- **Inter-Satellite Links** — Simulated optical/RF links between satellites based on line-of-sight and distance constraints
- **Ground Station Network** — ESA ground stations (Kiruna, Redu, Cebreros, Maspalomas, Kourou, New Norcia) with downlink visualization
- **Experiment Configurations** — Predefined mission scenarios (LEO Data Relay, Disaster Response, MEO-GEO Hybrid Routing, Blockchain Provenance)
- **Pass Predictions** — Calculate satellite visibility windows over ground stations
- **Mission Dashboard** — Overview of active experiments with statistics and status tracking

## Tech Stack

- **Frontend:** React 18, TypeScript, Vite
- **3D Engine:** CesiumJS with vite-plugin-cesium
- **Orbital Mechanics:** satellite.js (SGP4/SDP4 propagation)
- **UI:** Tailwind CSS, shadcn/ui, Lucide icons
- **Backend:** Lovable Cloud (authentication, database)
- **State Management:** TanStack Query

## Project Structure

```
src/
├── components/
│   ├── visualization/    # CesiumScene, satellite/ground station popups, search
│   ├── dashboard/        # Experiment cards, stats, header
│   ├── landing/          # Hero, use cases, technology sections
│   └── ui/               # shadcn/ui components
├── lib/
│   ├── experiment-configs.ts   # Mission scenario definitions
│   └── tle-service.ts          # TLE parsing, orbital propagation, pass prediction
├── pages/
│   ├── Index.tsx               # Landing page
│   ├── Dashboard.tsx           # Mission control dashboard
│   └── Visualization.tsx       # 3D globe view
└── integrations/
    └── supabase/               # Backend client & types
```

## Routes

| Path | Description |
|------|-------------|
| `/` | Landing page |
| `/dashboard` | Mission control dashboard |
| `/visualization` | 3D globe with all satellites |
| `/visualization/:experimentId` | Globe filtered to a specific experiment |

## Getting Started

```sh
git clone <YOUR_GIT_URL>
cd <YOUR_PROJECT_NAME>
npm install
npm run dev
```

## License

Proprietary — ESA Space Operations
