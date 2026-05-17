// Friendly cadence ↔ RRULE conversion. Covers the three shapes the
// PublicationForm UI offers (daily, weekly, monthly); anything else is
// treated as "custom" and round-tripped as a raw RRULE string.

export type Frequency = "none" | "daily" | "weekly" | "monthly" | "custom";

export const DAYS = [
  { value: "MO", short: "Mon" },
  { value: "TU", short: "Tue" },
  { value: "WE", short: "Wed" },
  { value: "TH", short: "Thu" },
  { value: "FR", short: "Fri" },
  { value: "SA", short: "Sat" },
  { value: "SU", short: "Sun" },
] as const;
export type DayCode = (typeof DAYS)[number]["value"];

export interface CadenceState {
  frequency: Frequency;
  hour: number;     // 0-23, used by daily/weekly/monthly
  minute: number;   // 0-59
  byDay: DayCode[]; // weekly only
  monthDay: number; // monthly only (1-31)
  customRule: string; // custom only
}

export function defaultCadence(): CadenceState {
  return {
    frequency: "none",
    hour: 9,
    minute: 0,
    byDay: ["MO"],
    monthDay: 1,
    customRule: "",
  };
}

// Build the RRULE string the backend stores. Returns null when the
// frequency is "none" (the form treats null as "unset cadence").
export function buildRRULE(state: CadenceState): string | null {
  const { frequency, hour, minute } = state;
  if (frequency === "none") return null;
  if (frequency === "custom") return state.customRule.trim() || null;

  const time = `BYHOUR=${hour};BYMINUTE=${minute};BYSECOND=0`;
  switch (frequency) {
    case "daily":
      return `FREQ=DAILY;${time}`;
    case "weekly": {
      const days = state.byDay.length ? state.byDay.join(",") : "MO";
      return `FREQ=WEEKLY;BYDAY=${days};${time}`;
    }
    case "monthly":
      return `FREQ=MONTHLY;BYMONTHDAY=${state.monthDay};${time}`;
  }
}

// Best-effort parse an RRULE back into UI state. Falls back to "custom"
// mode (preserving the raw rule) for anything we don't recognise — covers
// FREQ=YEARLY, INTERVAL=, BYSETPOS=, etc., without losing the existing rule.
export function parseRRULE(rule: string | null): CadenceState {
  const state = defaultCadence();
  if (!rule || !rule.trim()) return state;

  const parts: Record<string, string> = {};
  for (const segment of rule.trim().split(";")) {
    const [k, v] = segment.split("=");
    if (k && v !== undefined) parts[k.toUpperCase()] = v.toUpperCase();
  }

  state.hour = clamp(parseInt(parts.BYHOUR ?? "9", 10), 0, 23, 9);
  state.minute = clamp(parseInt(parts.BYMINUTE ?? "0", 10), 0, 59, 0);

  switch (parts.FREQ) {
    case "DAILY":
      if (Object.keys(parts).length <= 4) {
        state.frequency = "daily";
        return state;
      }
      break;
    case "WEEKLY":
      if (parts.BYDAY && /^[A-Z]{2}(,[A-Z]{2})*$/.test(parts.BYDAY)) {
        const days = parts.BYDAY.split(",") as DayCode[];
        if (days.every((d) => DAYS.some((day) => day.value === d))) {
          state.frequency = "weekly";
          state.byDay = days;
          return state;
        }
      }
      break;
    case "MONTHLY":
      if (parts.BYMONTHDAY && /^\d{1,2}$/.test(parts.BYMONTHDAY)) {
        const day = clamp(parseInt(parts.BYMONTHDAY, 10), 1, 31, 1);
        state.frequency = "monthly";
        state.monthDay = day;
        return state;
      }
      break;
  }

  state.frequency = "custom";
  state.customRule = rule;
  return state;
}

function clamp(n: number, lo: number, hi: number, fallback: number): number {
  if (Number.isNaN(n) || n < lo || n > hi) return fallback;
  return n;
}

// Human-readable preview shown under the form.
export function describe(state: CadenceState, tz: string): string {
  if (state.frequency === "none") return "No automatic schedule (ad-hoc only).";
  if (state.frequency === "custom") {
    return state.customRule ? `Custom: ${state.customRule}` : "Custom (not set).";
  }
  const time = formatTime(state.hour, state.minute);
  const tzLabel = tz || "UTC";
  switch (state.frequency) {
    case "daily":
      return `Every day at ${time} ${tzLabel}.`;
    case "weekly": {
      const days = state.byDay.length === 7
        ? "every day"
        : state.byDay
            .map((d) => DAYS.find((day) => day.value === d)?.short ?? d)
            .join(", ");
      return `Every week on ${days} at ${time} ${tzLabel}.`;
    }
    case "monthly": {
      const suffix = ordinal(state.monthDay);
      return `Every month on the ${state.monthDay}${suffix} at ${time} ${tzLabel}.`;
    }
  }
  return "";
}

function formatTime(h: number, m: number): string {
  const hh = String(h).padStart(2, "0");
  const mm = String(m).padStart(2, "0");
  return `${hh}:${mm}`;
}

function ordinal(n: number): string {
  const v = n % 100;
  if (v >= 11 && v <= 13) return "th";
  switch (n % 10) {
    case 1: return "st";
    case 2: return "nd";
    case 3: return "rd";
    default: return "th";
  }
}
