import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import SatelliteSearch from "@/components/visualization/SatelliteSearch";
import type { SatelliteSearchItem } from "@/components/visualization/SatelliteSearch";
import { DEFAULT_ORBIT_LAYER_VISIBILITY } from "@/lib/orbit-layers";

const baseSats: SatelliteSearchItem[] = [
  {
    id: "leo-1",
    name: "Alpha LEO",
    lat: 14.2,
    lon: 1,
    color: "#4ade80",
    orbitType: "LEO",
    altitude: 400,
    inclination: 98,
  },
  {
    id: "geo-1",
    name: "Beta GEO",
    lat: 0,
    lon: 0,
    color: "#f97316",
    orbitType: "GEO",
    altitude: 35786,
    inclination: 0.1,
  },
];

function openPanel() {
  fireEvent.click(screen.getByRole("button", { name: /search satellites/i }));
}

describe("SatelliteSearch", () => {
  it("shows empty copy when there are no satellites", () => {
    const onSelect = vi.fn();
    render(
      <SatelliteSearch
        satellites={[]}
        orbitLayerVisibility={DEFAULT_ORBIT_LAYER_VISIBILITY}
        onSelectSatellite={onSelect}
      />
    );
    openPanel();
    expect(screen.getByText("No satellites in this experiment")).toBeInTheDocument();
  });

  it("filters by name and updates footer count", () => {
    const onSelect = vi.fn();
    render(
      <SatelliteSearch
        satellites={baseSats}
        orbitLayerVisibility={DEFAULT_ORBIT_LAYER_VISIBILITY}
        onSelectSatellite={onSelect}
      />
    );
    openPanel();
    const input = screen.getByPlaceholderText(/search by name/i);
    fireEvent.change(input, { target: { value: "Beta" } });
    expect(screen.getByText(/1 of 2 satellites/i)).toBeInTheDocument();
    expect(screen.getByText("Beta GEO")).toBeInTheDocument();
    expect(screen.queryByText("Alpha LEO")).not.toBeInTheDocument();
  });

  it("hides satellites when their orbit layer is off", () => {
    const onSelect = vi.fn();
    const visibility = { ...DEFAULT_ORBIT_LAYER_VISIBILITY, LEO: false };
    render(
      <SatelliteSearch
        satellites={baseSats}
        orbitLayerVisibility={visibility}
        onSelectSatellite={onSelect}
      />
    );
    openPanel();
    expect(screen.queryByText("Alpha LEO")).not.toBeInTheDocument();
    expect(screen.getByText("Beta GEO")).toBeInTheDocument();
  });

  it("calls onSelectSatellite and closes panel", () => {
    const onSelect = vi.fn();
    render(
      <SatelliteSearch
        satellites={baseSats}
        orbitLayerVisibility={DEFAULT_ORBIT_LAYER_VISIBILITY}
        onSelectSatellite={onSelect}
      />
    );
    openPanel();
    const row = screen.getByRole("button", { name: /Alpha LEO/i });
    fireEvent.click(row);
    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "leo-1",
        name: "Alpha LEO",
        orbitType: "LEO",
        altitude: 400,
        inclination: 98,
        color: "#4ade80",
        lat: 14.2,
        lon: 1,
      })
    );
    const input = screen.queryByPlaceholderText(/search by name/i);
    expect(input).not.toBeInTheDocument();
  });

  it("clears search via the clear control", () => {
    const onSelect = vi.fn();
    render(
      <SatelliteSearch
        satellites={baseSats}
        orbitLayerVisibility={DEFAULT_ORBIT_LAYER_VISIBILITY}
        onSelectSatellite={onSelect}
      />
    );
    openPanel();
    const input = screen.getByPlaceholderText(/search by name/i) as HTMLInputElement;
    fireEvent.change(input, { target: { value: "nomatch" } });
    expect(screen.getByText("No satellites found")).toBeInTheDocument();
    const panel = screen.getByPlaceholderText(/search by name/i).closest(".shadow-xl");
    const clearBtn = within(panel as HTMLElement).getByRole("button", { hidden: true });
    fireEvent.click(clearBtn);
    expect((screen.getByPlaceholderText(/search by name/i) as HTMLInputElement).value).toBe("");
  });
});
