import { useState, useMemo } from "react";
import { Button } from "@/components/ui/button";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import { Clock, Radio, ArrowUp, Compass } from "lucide-react";
import { sampleTLEs, parseTLE, predictPasses, PassPrediction as PassPredictionType } from "@/lib/tle-service";

interface GroundStation {
  name: string;
  lat: number;
  lng: number;
}

interface PassPredictionProps {
  groundStations: GroundStation[];
  baseTime: Date;
}

const PassPrediction = ({ groundStations, baseTime }: PassPredictionProps) => {
  const [selectedStation, setSelectedStation] = useState<string>(groundStations[0]?.name || "");
  const [selectedSatellite, setSelectedSatellite] = useState<string>(sampleTLEs[0]?.name || "");

  const station = useMemo(
    () => groundStations.find((s) => s.name === selectedStation),
    [groundStations, selectedStation]
  );

  const tle = useMemo(
    () => sampleTLEs.find((t) => t.name === selectedSatellite),
    [selectedSatellite]
  );

  const passes = useMemo(() => {
    if (!station || !tle) return [];
    
    const satrec = parseTLE(tle);
    if (!satrec) return [];
    
    return predictPasses(
      satrec,
      { lat: station.lat, lng: station.lng },
      baseTime,
      48 // 48 hours ahead
    );
  }, [station, tle, baseTime]);

  const formatTime = (date: Date) => {
    return date.toLocaleTimeString("en-US", {
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    });
  };

  const formatDate = (date: Date) => {
    return date.toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
    });
  };

  const getElevationColor = (elevation: number) => {
    if (elevation >= 60) return "text-esa-success";
    if (elevation >= 30) return "text-yellow-500";
    return "text-orange-500";
  };

  const getElevationLabel = (elevation: number) => {
    if (elevation >= 60) return "Excellent";
    if (elevation >= 30) return "Good";
    return "Low";
  };

  return (
    <div className="space-y-4">
      <div className="space-y-3">
        <div className="space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">Ground Station</label>
          <Select value={selectedStation} onValueChange={setSelectedStation}>
            <SelectTrigger className="h-9">
              <SelectValue placeholder="Select station" />
            </SelectTrigger>
            <SelectContent>
              {groundStations.map((station) => (
                <SelectItem key={station.name} value={station.name}>
                  <div className="flex items-center gap-2">
                    <Radio className="h-3 w-3 text-accent" />
                    {station.name}
                  </div>
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">Satellite</label>
          <Select value={selectedSatellite} onValueChange={setSelectedSatellite}>
            <SelectTrigger className="h-9">
              <SelectValue placeholder="Select satellite" />
            </SelectTrigger>
            <SelectContent>
              {sampleTLEs.map((tle) => (
                <SelectItem key={tle.name} value={tle.name}>
                  {tle.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>

      <div className="border-t border-border pt-3">
        <h4 className="text-sm font-semibold mb-3 flex items-center gap-2">
          <Clock className="h-4 w-4 text-primary" />
          Upcoming Passes (48h)
        </h4>

        {passes.length === 0 ? (
          <p className="text-xs text-muted-foreground text-center py-4">
            No visible passes in the next 48 hours
          </p>
        ) : (
          <ScrollArea className="h-[280px]">
            <div className="space-y-2 pr-2">
              {passes.map((pass, index) => (
                <PassCard key={index} pass={pass} formatTime={formatTime} formatDate={formatDate} getElevationColor={getElevationColor} getElevationLabel={getElevationLabel} />
              ))}
            </div>
          </ScrollArea>
        )}
      </div>
    </div>
  );
};

interface PassCardProps {
  pass: PassPredictionType;
  formatTime: (date: Date) => string;
  formatDate: (date: Date) => string;
  getElevationColor: (el: number) => string;
  getElevationLabel: (el: number) => string;
}

const PassCard = ({ pass, formatTime, formatDate, getElevationColor, getElevationLabel }: PassCardProps) => {
  const duration = Math.round((pass.endTime.getTime() - pass.startTime.getTime()) / 60000);
  
  return (
    <div className="p-3 rounded-lg bg-secondary/50 space-y-2">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium">
          {formatDate(pass.startTime)}
        </span>
        <Badge variant="outline" className={`${getElevationColor(pass.maxElevation)} text-[10px]`}>
          {getElevationLabel(pass.maxElevation)}
        </Badge>
      </div>
      
      <div className="flex items-center gap-4 text-xs">
        <div className="flex items-center gap-1.5">
          <Clock className="h-3 w-3 text-muted-foreground" />
          <span>{formatTime(pass.startTime)} - {formatTime(pass.endTime)}</span>
        </div>
        <span className="text-muted-foreground">({duration} min)</span>
      </div>
      
      <div className="flex items-center justify-between text-xs">
        <div className="flex items-center gap-1.5">
          <ArrowUp className="h-3 w-3 text-muted-foreground" />
          <span className={getElevationColor(pass.maxElevation)}>
            {pass.maxElevation.toFixed(1)}° max
          </span>
        </div>
        <div className="flex items-center gap-1.5 text-muted-foreground">
          <Compass className="h-3 w-3" />
          <span>{pass.azimuthStart.toFixed(0)}° → {pass.azimuthEnd.toFixed(0)}°</span>
        </div>
      </div>
    </div>
  );
};

export default PassPrediction;
