import { AlertTriangle, Radio, Route } from "lucide-react";

const useCases = [
  {
    id: "UC1",
    icon: <AlertTriangle className="h-6 w-6" />,
    title: "Rapid Disaster Response",
    description: "Quick access to EO data in the aftermath of natural disasters. Decentralized storage facilitates faster data dissemination to emergency responders, even when traditional communication infrastructures are compromised.",
    color: "esa-warning",
  },
  {
    id: "UC2",
    icon: <Radio className="h-6 w-6" />,
    title: "Continuous Line of Sight Relay",
    description: "Satellites relay data amongst themselves, creating a mesh network that enhances data availability and reduces latency. Particularly relevant for constellations of small satellites with limited ground station access.",
    color: "esa-cyan",
  },
  {
    id: "UC3",
    icon: <Route className="h-6 w-6" />,
    title: "Situation-Aware Data Routing",
    description: "Decentralized storage enables intelligent data routing based on situational awareness, prioritizing certain data types during specific events like environmental monitoring during wildfires.",
    color: "esa-purple",
  },
];

const UseCasesSection = () => {
  return (
    <section className="py-24 relative">
      <div className="absolute inset-0 data-grid opacity-30" />
      
      <div className="container mx-auto px-6 relative z-10">
        <div className="text-center mb-16">
          <span className="text-primary text-sm font-medium uppercase tracking-wider">Research Focus</span>
          <h2 className="text-3xl md:text-4xl font-bold mt-4 mb-6">Use Cases Under Investigation</h2>
          <p className="text-muted-foreground max-w-2xl mx-auto">
            Exploring practical applications of decentralized technologies for Earth Observation data management.
          </p>
        </div>

        <div className="grid md:grid-cols-3 gap-8">
          {useCases.map((useCase, index) => (
            <div
              key={useCase.id}
              className="glass-card p-8 aurora-border hover:scale-[1.02] transition-transform duration-300"
              style={{ animationDelay: `${index * 0.1}s` }}
            >
              <div className="flex items-center gap-4 mb-6">
                <div className={`p-3 rounded-lg bg-${useCase.color}/10 text-${useCase.color}`}>
                  {useCase.icon}
                </div>
                <span className="text-xs font-mono text-muted-foreground uppercase tracking-wider">
                  {useCase.id}
                </span>
              </div>
              
              <h3 className="text-xl font-semibold mb-4">{useCase.title}</h3>
              <p className="text-muted-foreground text-sm leading-relaxed">
                {useCase.description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
};

export default UseCasesSection;
