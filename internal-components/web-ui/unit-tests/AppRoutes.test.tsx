import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { TooltipProvider } from "@/components/ui/tooltip";
import { AppRoutes } from "@/App";

vi.mock("@/pages/Index", () => ({
  default: () => <div data-testid="route-index">index</div>,
}));
vi.mock("@/pages/Dashboard", () => ({
  default: () => <div data-testid="route-dashboard">dashboard</div>,
}));
vi.mock("@/pages/Visualization", () => ({
  default: () => <div data-testid="route-visualization">visualization</div>,
}));
vi.mock("@/pages/NotFound", () => ({
  default: () => <div data-testid="route-not-found">not-found</div>,
}));

function renderAt(path: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <TooltipProvider>
        <MemoryRouter initialEntries={[path]}>
          <AppRoutes />
        </MemoryRouter>
      </TooltipProvider>
    </QueryClientProvider>
  );
}

describe("AppRoutes", () => {
  it("renders / as Index", () => {
    renderAt("/");
    expect(screen.getByTestId("route-index")).toBeInTheDocument();
  });

  it("renders /dashboard as Dashboard", () => {
    renderAt("/dashboard");
    expect(screen.getByTestId("route-dashboard")).toBeInTheDocument();
  });

  it("renders /visualization as Visualization", () => {
    renderAt("/visualization");
    expect(screen.getByTestId("route-visualization")).toBeInTheDocument();
  });

  it("renders /visualization/:experimentId as Visualization", () => {
    renderAt("/visualization/EXP-001");
    expect(screen.getByTestId("route-visualization")).toBeInTheDocument();
  });

  it("renders unknown paths as NotFound", () => {
    renderAt("/does-not-exist");
    expect(screen.getByTestId("route-not-found")).toBeInTheDocument();
  });
});
