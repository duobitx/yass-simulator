import { useMemo, useState } from "react";
import { formatDistanceToNow } from "date-fns";
import { useNavigate } from "react-router-dom";
import DashboardHeader from "@/components/dashboard/DashboardHeader";
import ExperimentCard from "@/components/dashboard/ExperimentCard";
import StatsCard from "@/components/dashboard/StatsCard";
import Footer from "@/components/landing/Footer";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Satellite,
  Database,
  Activity,
  Search,
  Filter,
  Globe,
} from "lucide-react";
import { useSatelliteSse } from "@/hooks/useSatelliteSse";
import { sseEventsUrl, isGroundStationEvent } from "@/lib/sse-types";
import { experimentConfigs } from "@/lib/experiment-configs";

const LIVE_EXPERIMENT = {
  id: "EXP-001",
  name: "Satellite Network Live",
  status: "running" as const,
  duration: "Live",
};

const Dashboard = () => {
  const [searchQuery, setSearchQuery] = useState("");
  const navigate = useNavigate();
  const { tracks, status } = useSatelliteSse(sseEventsUrl(), true);

  const satelliteCount = useMemo(
    () => Object.keys(tracks).filter((id) => !isGroundStationEvent(tracks[id])).length,
    [tracks]
  );
  const groundCount = useMemo(
    () => Object.keys(tracks).filter((id) => isGroundStationEvent(tracks[id])).length,
    [tracks]
  );
  const trackTotal = Object.keys(tracks).length;

  const lastUpdatedLabel = useMemo(() => {
    const times = Object.values(tracks)
      .map((t) => new Date(t.timestamp).getTime())
      .filter((n) => !Number.isNaN(n));
    if (!times.length) {
      if (status === "connecting") return "Connecting…";
      if (status === "error") return "Stream error";
      return "Waiting for data…";
    }
    const latest = Math.max(...times);
    return formatDistanceToNow(new Date(latest), { addSuffix: true });
  }, [tracks, status]);

  const experiments = useMemo(() => {
    const demo = experimentConfigs["EXP-DEMO"];
    return [
      {
        ...LIVE_EXPERIMENT,
        satellites: satelliteCount,
        groundStations: groundCount,
        lastUpdated: lastUpdatedLabel,
      },
      {
        id: demo.id,
        name: demo.name,
        status: "running" as const,
        satellites: demo.satellites.length,
        groundStations: demo.groundStations.length,
        duration: "Simulation",
        lastUpdated: "Static catalog",
      },
    ];
  }, [satelliteCount, groundCount, lastUpdatedLabel]);

  const streamSubtitle =
    status === "live"
      ? "Receiving positions"
      : status === "connecting"
        ? "Connecting to stream…"
        : status === "error"
          ? "Check backend / proxy"
          : "Idle";

  const filteredExperiments = experiments.filter(
    (exp) =>
      exp.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      exp.id.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="min-h-screen bg-background">
      <DashboardHeader />

      <main className="container mx-auto px-6 py-8">
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6 mb-8">
          <StatsCard
            title="Active Experiments"
            value={experiments.length}
            subtitle="Currently running"
            icon={Activity}
          />
          <StatsCard
            title="Satellites (stream)"
            value={satelliteCount}
            subtitle={status === "live" ? "Live positions" : streamSubtitle}
            icon={Satellite}
          />
          <StatsCard
            title="Ground segment"
            value={groundCount}
            subtitle="Alt = 0 on stream"
            icon={Globe}
          />
          <StatsCard
            title="Tracked sources"
            value={trackTotal}
            subtitle={streamSubtitle}
            icon={Database}
          />
        </div>

        <div className="mb-8">
          <div className="flex flex-col sm:flex-row justify-between items-start sm:items-center gap-4 mb-6">
            <div>
              <h2 className="text-2xl font-bold">Experiments</h2>
              <p className="text-muted-foreground text-sm">
                Manage and visualize your EO data experiments
              </p>
            </div>
          </div>

          <div className="flex gap-4 mb-6">
            <div className="relative flex-1 max-w-md">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search experiments..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-10 bg-secondary/50"
              />
            </div>
            <Button variant="outline" className="flex items-center gap-2">
              <Filter className="h-4 w-4" />
              Filter
            </Button>
          </div>

          <div className="grid md:grid-cols-2 lg:grid-cols-3 gap-6">
            {filteredExperiments.map((experiment) => (
              <ExperimentCard
                key={experiment.id}
                {...experiment}
                onView={() => navigate(`/visualization/${experiment.id}`)}
              />
            ))}
          </div>

          {filteredExperiments.length === 0 && (
            <div className="glass-card p-12 text-center">
              <p className="text-muted-foreground">
                No experiments found matching your search.
              </p>
            </div>
          )}
        </div>
      </main>

      <Footer />
    </div>
  );
};

export default Dashboard;
