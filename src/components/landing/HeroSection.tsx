import { Button } from "@/components/ui/button";
import { ArrowRight, Satellite, Globe, Database } from "lucide-react";
import { Link } from "react-router-dom";
import esaLogo from "@/assets/esa-logo.svg";

const HeroSection = () => {
  return (
    <section className="relative min-h-screen flex items-center justify-center overflow-hidden star-field">
      {/* Background gradient overlay */}
      <div className="absolute inset-0 bg-gradient-to-b from-background via-background/95 to-secondary/20" />
      
      {/* Animated orbital rings */}
      <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[800px] h-[800px] opacity-20">
        <div className="absolute inset-0 border border-primary/30 rounded-full animate-orbit" style={{ animationDuration: '30s' }} />
        <div className="absolute inset-8 border border-accent/20 rounded-full animate-orbit" style={{ animationDuration: '25s', animationDirection: 'reverse' }} />
        <div className="absolute inset-16 border border-primary/20 rounded-full animate-orbit" style={{ animationDuration: '20s' }} />
      </div>

      {/* Content */}
      <div className="relative z-10 container mx-auto px-6 text-center">
        {/* ESA Logo */}
        <div className="mb-8 flex justify-center animate-fade-in">
          <img src={esaLogo} alt="ESA Logo" className="h-16 w-auto" />
        </div>

        {/* Badge */}
        <div className="inline-flex items-center gap-2 px-4 py-2 rounded-full bg-secondary/50 border border-border/50 mb-8 animate-fade-in" style={{ animationDelay: '0.1s' }}>
          <span className="w-2 h-2 rounded-full bg-esa-success animate-pulse" />
          <span className="text-sm text-muted-foreground">ESA Φ-lab R&D Project</span>
        </div>

        {/* Title */}
        <h1 className="text-4xl md:text-6xl lg:text-7xl font-bold mb-6 animate-fade-in" style={{ animationDelay: '0.2s' }}>
          <span className="block text-foreground">Decentralized Storage</span>
          <span className="block gradient-text mt-2">& Provenance of EO Data</span>
        </h1>

        {/* Subtitle */}
        <p className="text-lg md:text-xl text-muted-foreground max-w-3xl mx-auto mb-12 animate-fade-in" style={{ animationDelay: '0.3s' }}>
          Exploring disruptive technologies for secure, transparent, and trustworthy 
          Earth Observation data management through decentralized solutions.
        </p>

        {/* CTA Buttons */}
        <div className="flex flex-col sm:flex-row gap-4 justify-center items-center animate-fade-in" style={{ animationDelay: '0.4s' }}>
          <Link to="/dashboard">
            <Button size="lg" className="group bg-primary hover:bg-primary/90 text-primary-foreground px-8 py-6 text-lg">
              Access Dashboard
              <ArrowRight className="ml-2 h-5 w-5 transition-transform group-hover:translate-x-1" />
            </Button>
          </Link>
          <Button size="lg" variant="outline" className="px-8 py-6 text-lg border-border/50 hover:bg-secondary/50">
            View Documentation
          </Button>
        </div>

        {/* Feature icons */}
        <div className="mt-20 grid grid-cols-1 md:grid-cols-3 gap-8 max-w-4xl mx-auto animate-fade-in" style={{ animationDelay: '0.5s' }}>
          <FeatureCard
            icon={<Satellite className="h-8 w-8" />}
            title="Satellite Networks"
            description="Real-time visualization of orbital constellations and data flows"
          />
          <FeatureCard
            icon={<Database className="h-8 w-8" />}
            title="Decentralized Storage"
            description="Distributed ledger technology for EO data integrity"
          />
          <FeatureCard
            icon={<Globe className="h-8 w-8" />}
            title="Global Coverage"
            description="Comprehensive Earth observation data accessibility"
          />
        </div>
      </div>
    </section>
  );
};

const FeatureCard = ({ icon, title, description }: { icon: React.ReactNode; title: string; description: string }) => (
  <div className="glass-card p-6 text-center hover:border-primary/50 transition-colors">
    <div className="inline-flex items-center justify-center w-16 h-16 rounded-xl bg-primary/10 text-primary mb-4">
      {icon}
    </div>
    <h3 className="text-lg font-semibold mb-2">{title}</h3>
    <p className="text-sm text-muted-foreground">{description}</p>
  </div>
);

export default HeroSection;
