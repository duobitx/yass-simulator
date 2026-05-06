import { Shield, Link as LinkIcon, Cloud, Cpu } from "lucide-react";

const technologies = [
  {
    icon: <Shield className="h-6 w-6" />,
    title: "Data Integrity & Provenance",
    description: "Ensuring traceability of EO data from source to end-users with comprehensive metadata tracking.",
  },
  {
    icon: <LinkIcon className="h-6 w-6" />,
    title: "Distributed Ledger Technology",
    description: "Blockchain-based solutions for secure, transparent, and immutable data records.",
  },
  {
    icon: <Cloud className="h-6 w-6" />,
    title: "Cognitive Cloud in Space",
    description: "Moving data processing and storage from ground to space infrastructure.",
  },
  {
    icon: <Cpu className="h-6 w-6" />,
    title: "AI4EO Integration",
    description: "Machine learning techniques adapted to work with geospatial data.",
  },
];

const TechnologySection = () => {
  return (
    <section className="py-24 bg-secondary/20">
      <div className="container mx-auto px-6">
        <div className="grid lg:grid-cols-2 gap-16 items-center">
          {/* Left side - Text */}
          <div>
            <span className="text-primary text-sm font-medium uppercase tracking-wider">
              Technology Stack
            </span>
            <h2 className="text-3xl md:text-4xl font-bold mt-4 mb-6">
              Disruptive Technologies for EO
            </h2>
            <p className="text-muted-foreground mb-8 leading-relaxed">
              The activity aims to carry out an exploratory study and develop decentralized 
              and secure storage systems, as well as integrity and provenance mechanisms 
              for Earth Observation data. Aligned with ESA Φ-lab strategic objectives.
            </p>

            <div className="space-y-4">
              {technologies.map((tech, index) => (
                <div
                  key={index}
                  className="flex items-start gap-4 p-4 rounded-lg hover:bg-card/50 transition-colors"
                >
                  <div className="p-2 rounded-lg bg-primary/10 text-primary flex-shrink-0">
                    {tech.icon}
                  </div>
                  <div>
                    <h4 className="font-semibold mb-1">{tech.title}</h4>
                    <p className="text-sm text-muted-foreground">{tech.description}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>

          {/* Right side - Visual */}
          <div className="relative">
            <div className="glass-card p-8 aurora-border">
              <div className="aspect-square relative flex items-center justify-center">
                {/* Central node */}
                <div className="absolute w-20 h-20 rounded-full bg-primary/20 border-2 border-primary flex items-center justify-center animate-glow-pulse">
                  <span className="text-primary font-mono text-sm">EO</span>
                </div>
                
                {/* Orbital nodes */}
                {[0, 60, 120, 180, 240, 300].map((angle, i) => (
                  <div
                    key={i}
                    className="absolute w-12 h-12 rounded-full bg-secondary border border-border flex items-center justify-center"
                    style={{
                      transform: `rotate(${angle}deg) translateX(120px) rotate(-${angle}deg)`,
                    }}
                  >
                    <span className="text-xs font-mono text-muted-foreground">N{i + 1}</span>
                  </div>
                ))}
                
                {/* Connection lines */}
                <svg className="absolute inset-0 w-full h-full" viewBox="0 0 100 100">
                  <circle
                    cx="50"
                    cy="50"
                    r="35"
                    fill="none"
                    stroke="hsl(var(--border))"
                    strokeWidth="0.5"
                    strokeDasharray="4 2"
                    className="animate-orbit"
                    style={{ animationDuration: '40s' }}
                  />
                </svg>
              </div>
              
              <div className="mt-6 pt-6 border-t border-border">
                <div className="flex justify-between text-sm">
                  <div>
                    <span className="text-muted-foreground">TRL Target</span>
                    <p className="font-mono text-primary">5/6</p>
                  </div>
                  <div className="text-right">
                    <span className="text-muted-foreground">Topics</span>
                    <p className="font-mono text-primary">T1 & T2</p>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </section>
  );
};

export default TechnologySection;
