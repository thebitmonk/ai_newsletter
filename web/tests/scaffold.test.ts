// Smoke test — proves the vitest harness boots inside the Nuxt app dir.
// Comprehensive composable tests (F2/F3 per PRD #12) land alongside the
// auth pages in slice #16, where mocked Firebase / fetch exercise them end-to-end.
import { describe, expect, it } from "vitest";

describe("vitest scaffold", () => {
  it("runs", () => {
    expect(1 + 1).toBe(2);
  });
});
