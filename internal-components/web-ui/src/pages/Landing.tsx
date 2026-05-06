import HeroSection from "@/components/landing/HeroSection";
import UseCasesSection from "@/components/landing/UseCasesSection";
import TechnologySection from "@/components/landing/TechnologySection";
import Footer from "@/components/landing/Footer";

const Landing = () => {
  return (
    <div className="min-h-screen bg-background">
      <HeroSection />
      <UseCasesSection />
      <TechnologySection />
      <Footer />
    </div>
  );
};

export default Landing;
