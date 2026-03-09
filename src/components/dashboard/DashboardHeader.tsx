import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Bell, Settings } from "lucide-react";
import esaLogo from "@/assets/esa-logo.svg";

const DashboardHeader = () => {
  return (
    <header className="h-16 border-b border-border bg-card/80 backdrop-blur-xl sticky top-0 z-50">
      <div className="container mx-auto px-6 h-full flex items-center justify-between">
        <div className="flex items-center gap-4">
          <Link to="/"><img src={esaLogo} alt="ESA" className="h-8 w-auto" /></Link>
          <div className="hidden md:block">
            <h1 className="text-sm font-semibold">EO Data Dashboard</h1>
            <p className="text-xs text-muted-foreground">Decentralized Storage R&D</p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground">
            <Bell className="h-5 w-5" />
          </Button>
          <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground">
            <Settings className="h-5 w-5" />
          </Button>
        </div>
      </div>
    </header>
  );
};

export default DashboardHeader;
