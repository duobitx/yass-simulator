import { describe, it, expect } from "vitest";
import { cn } from "@/lib/utils";

describe("cn", () => {
  it("merges tailwind classes and resolves conflicts", () => {
    expect(cn("px-2 py-1", "px-4")).toBe("py-1 px-4");
  });

  it("handles conditional and array inputs", () => {
    const showHidden: boolean = false;
    expect(cn("base", showHidden && "hidden", ["ring-2", "ring-offset-2"])).toBe(
      "base ring-2 ring-offset-2"
    );
  });
});
