import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Play, Clock, Satellite, Globe } from "lucide-react";

interface ExperimentCardProps {
  id: string;
  name: string;
  status: "running" | "completed" | "pending";
  satellites: number;
  groundStations: number;
  duration: string;
  lastUpdated: string;
  onView: () => void;
}

const statusStyles = {
  running: "bg-esa-success/20 text-esa-success border-esa-success/30",
  completed: "bg-primary/20 text-primary border-primary/30",
  pending: "bg-esa-warning/20 text-esa-warning border-esa-warning/30",
};

const ExperimentCard = ({
  id,
  name,
  status,
  satellites,
  groundStations,
  duration,
  lastUpdated,
  onView,
}: ExperimentCardProps) => {
  return (
    <div className="glass-card p-6 hover:border-primary/30 transition-all duration-300 group">
      <div className="flex items-start justify-between mb-4">
        <div>
          <span className="text-xs font-mono text-muted-foreground">{id}</span>
          <h3 className="text-lg font-semibold mt-1 group-hover:text-primary transition-colors">
            {name}
          </h3>
        </div>
        <Badge variant="outline" className={statusStyles[status]}>
          {status}
        </Badge>
      </div>

      <div className="grid grid-cols-2 gap-4 mb-6">
        <div className="flex items-center gap-2 text-sm">
          <Satellite className="h-4 w-4 text-muted-foreground" />
          <span>{satellites} Satellites</span>
        </div>
        <div className="flex items-center gap-2 text-sm">
          <Globe className="h-4 w-4 text-muted-foreground" />
          <span>{groundStations} Ground Stations</span>
        </div>
        <div className="flex items-center gap-2 text-sm">
          <Clock className="h-4 w-4 text-muted-foreground" />
          <span>{duration}</span>
        </div>
        <div className="text-sm text-muted-foreground">
          Updated: {lastUpdated}
        </div>
      </div>

      <Button className="w-full" variant="secondary" onClick={onView}>
        <Play className="mr-2 h-4 w-4" />
        View Experiment
      </Button>
    </div>
  );
};

export default ExperimentCard;
