import { describe, expect, it } from "vitest";

import {
  buildRRULE,
  defaultCadence,
  describe as describeCadence,
  parseRRULE,
} from "../utils/cadence";

describe("buildRRULE", () => {
  it("returns null for the 'none' frequency", () => {
    expect(buildRRULE({ ...defaultCadence(), frequency: "none" })).toBeNull();
  });

  it("builds a DAILY rule with hour + minute", () => {
    expect(
      buildRRULE({ ...defaultCadence(), frequency: "daily", hour: 9, minute: 30 }),
    ).toBe("FREQ=DAILY;BYHOUR=9;BYMINUTE=30;BYSECOND=0");
  });

  it("builds a WEEKLY rule with BYDAY (joined)", () => {
    expect(
      buildRRULE({
        ...defaultCadence(),
        frequency: "weekly",
        hour: 9,
        minute: 0,
        byDay: ["MO", "WE", "FR"],
      }),
    ).toBe("FREQ=WEEKLY;BYDAY=MO,WE,FR;BYHOUR=9;BYMINUTE=0;BYSECOND=0");
  });

  it("falls back to MO when weekly has no days selected", () => {
    expect(
      buildRRULE({ ...defaultCadence(), frequency: "weekly", byDay: [] }),
    ).toContain("BYDAY=MO");
  });

  it("builds a MONTHLY rule with BYMONTHDAY", () => {
    expect(
      buildRRULE({
        ...defaultCadence(),
        frequency: "monthly",
        monthDay: 15,
        hour: 8,
        minute: 0,
      }),
    ).toBe("FREQ=MONTHLY;BYMONTHDAY=15;BYHOUR=8;BYMINUTE=0;BYSECOND=0");
  });

  it("returns the raw custom rule for custom mode", () => {
    expect(
      buildRRULE({
        ...defaultCadence(),
        frequency: "custom",
        customRule: "FREQ=YEARLY;BYMONTH=12;BYMONTHDAY=25",
      }),
    ).toBe("FREQ=YEARLY;BYMONTH=12;BYMONTHDAY=25");
  });
});

describe("parseRRULE → buildRRULE round-trip", () => {
  it("daily", () => {
    const s = parseRRULE("FREQ=DAILY;BYHOUR=9;BYMINUTE=0;BYSECOND=0");
    expect(s.frequency).toBe("daily");
    expect(s.hour).toBe(9);
    expect(buildRRULE(s)).toBe("FREQ=DAILY;BYHOUR=9;BYMINUTE=0;BYSECOND=0");
  });

  it("weekly with multiple days", () => {
    const s = parseRRULE("FREQ=WEEKLY;BYDAY=MO,WE,FR;BYHOUR=9;BYMINUTE=0;BYSECOND=0");
    expect(s.frequency).toBe("weekly");
    expect(s.byDay).toEqual(["MO", "WE", "FR"]);
    expect(buildRRULE(s)).toBe("FREQ=WEEKLY;BYDAY=MO,WE,FR;BYHOUR=9;BYMINUTE=0;BYSECOND=0");
  });

  it("monthly", () => {
    const s = parseRRULE("FREQ=MONTHLY;BYMONTHDAY=15;BYHOUR=8;BYMINUTE=30;BYSECOND=0");
    expect(s.frequency).toBe("monthly");
    expect(s.monthDay).toBe(15);
    expect(s.hour).toBe(8);
    expect(s.minute).toBe(30);
  });

  it("falls back to custom for unrecognised rules", () => {
    const rule = "FREQ=YEARLY;BYMONTH=12;BYMONTHDAY=25";
    const s = parseRRULE(rule);
    expect(s.frequency).toBe("custom");
    expect(s.customRule).toBe(rule);
    expect(buildRRULE(s)).toBe(rule);
  });

  it("returns the default (none) when parsing null/empty", () => {
    expect(parseRRULE(null).frequency).toBe("none");
    expect(parseRRULE("").frequency).toBe("none");
    expect(parseRRULE("  ").frequency).toBe("none");
  });
});

describe("describe", () => {
  it("renders 'No automatic schedule' for none", () => {
    expect(describeCadence(defaultCadence(), "UTC")).toMatch(/No automatic/i);
  });

  it("renders weekly with day list", () => {
    const out = describeCadence(
      { ...defaultCadence(), frequency: "weekly", byDay: ["MO", "TH"], hour: 9, minute: 0 },
      "America/New_York",
    );
    expect(out).toContain("Mon");
    expect(out).toContain("Thu");
    expect(out).toContain("09:00");
    expect(out).toContain("America/New_York");
  });

  it("renders monthly with ordinal", () => {
    expect(
      describeCadence(
        { ...defaultCadence(), frequency: "monthly", monthDay: 21, hour: 10, minute: 0 },
        "UTC",
      ),
    ).toMatch(/21st/);
  });
});
