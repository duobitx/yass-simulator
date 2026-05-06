import esaLogo from "@/assets/esa-logo.svg";

const Footer = () => {
  return (
    <footer className="py-12 border-t border-border">
      <div className="container mx-auto px-6">
        <div className="flex flex-col md:flex-row justify-between items-center gap-6">
          <div className="flex items-center gap-4">
            <img src={esaLogo} alt="ESA Logo" className="h-10 w-auto" />
            <div className="text-sm text-muted-foreground">
              <p>European Space Agency</p>
              <p>Φ-lab Research Project</p>
            </div>
          </div>
          
          <div className="text-sm text-muted-foreground text-center md:text-right">
            <p>ESA-EOPΦL-SOW-2024-0471</p>
            <p className="mt-1">Disruptive Technologies for Decentralized Storage and Provenance of EO Data</p>
          </div>
        </div>
        
        <div className="mt-8 pt-8 border-t border-border/50 text-center text-xs text-muted-foreground">
          <p>ESA UNCLASSIFIED – For ESA Official Use Only</p>
        </div>
      </div>
    </footer>
  );
};

export default Footer;
