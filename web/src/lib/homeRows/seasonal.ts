type Config = Record<string, unknown>;

/** A holiday a user adds to a Seasonal Picks row (the server's SeasonalCustomHoliday). */
export interface CustomHoliday {
  name: string;
  /** MM-DD, inclusive. An end before the start wraps across New Year. */
  start: string;
  end: string;
  list_url: string;
  title?: string;
  /** How the row dresses itself on the web home page in season. */
  appearance?: HolidayAppearance;
}

/**
 * How a Seasonal Picks row dresses itself on the web home page while one
 * holiday is in season (the server's SeasonalAppearance). Every field is
 * optional and the apps read none of it.
 */
export interface HolidayAppearance {
  /** Content reference of an uploaded background, not a URL. */
  background?: string;
  headline?: string;
  subtitle?: string;
  button_label?: string;
}

/** The row's theme_appearance, keyed by holiday, as saved. */
export function themeAppearanceOf(config: Config): Record<string, HolidayAppearance> {
  const out: Record<string, HolidayAppearance> = {};
  const raw = config.theme_appearance;
  if (!raw || typeof raw !== "object") return out;
  for (const [key, entry] of Object.entries(raw as Record<string, unknown>)) {
    const appearance = appearanceOf(entry);
    if (appearance) out[key] = appearance;
  }
  return out;
}

/** One appearance, read from stored JSON. Null when it says nothing. */
export function appearanceOf(value: unknown): HolidayAppearance | null {
  if (!value || typeof value !== "object") return null;
  const raw = value as Record<string, unknown>;
  const appearance: HolidayAppearance = {};
  if (typeof raw.background === "string") appearance.background = raw.background;
  if (typeof raw.headline === "string") appearance.headline = raw.headline;
  if (typeof raw.subtitle === "string") appearance.subtitle = raw.subtitle;
  if (typeof raw.button_label === "string") appearance.button_label = raw.button_label;
  return hasAppearance(appearance) ? appearance : null;
}

export function hasAppearance(appearance: HolidayAppearance | null | undefined): boolean {
  if (!appearance) return false;
  return Boolean(
    appearance.background?.trim() ||
    appearance.headline?.trim() ||
    appearance.subtitle?.trim() ||
    appearance.button_label?.trim(),
  );
}

/**
 * The appearance as the row stores it: fields that are blank (ignoring
 * surrounding whitespace) are left out, and an appearance left with nothing is
 * dropped entirely, so clearing a holiday removes its entry. Text keeps
 * whatever was typed — the server trims it when it serves the row, and trimming
 * here would eat the space between two words as it is typed.
 */
export function prunedAppearance(value: HolidayAppearance): HolidayAppearance | undefined {
  const appearance: HolidayAppearance = {};
  if (value.background?.trim()) appearance.background = value.background;
  if (value.headline?.trim()) appearance.headline = value.headline;
  if (value.subtitle?.trim()) appearance.subtitle = value.subtitle;
  if (value.button_label?.trim()) appearance.button_label = value.button_label;
  return hasAppearance(appearance) ? appearance : undefined;
}

/**
 * Where an uploaded background is served from. The reference is the hash of the
 * stored bytes, so the path is its own cache buster and an uploaded image can
 * be previewed before the row is saved.
 */
export function seasonalBackgroundURL(ref: string): string {
  return `/api/v2/seasonal-backgrounds/${encodeURIComponent(ref)}`;
}

export function customHolidaysOf(config: Config): CustomHoliday[] {
  if (!Array.isArray(config.custom_holidays)) return [];
  return config.custom_holidays
    .filter((entry): entry is Record<string, unknown> => !!entry && typeof entry === "object")
    .map((entry) => ({
      ...(entry as Partial<CustomHoliday>),
      name: typeof entry.name === "string" ? entry.name : "",
      start: typeof entry.start === "string" ? entry.start : "",
      end: typeof entry.end === "string" ? entry.end : "",
      list_url: typeof entry.list_url === "string" ? entry.list_url : "",
    }));
}

// Days per month in a leap year: Feb 29 is a valid boundary.
const MONTH_DAYS = [31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

function validMonthDay(value: string): boolean {
  const match = /^(\d{2})-(\d{2})$/.exec(value);
  if (!match) return false;
  const month = Number(match[1]);
  const day = Number(match[2]);
  return month >= 1 && month <= 12 && day >= 1 && day <= MONTH_DAYS[month - 1]!;
}

const MONTH_NAMES = [
  "Jan",
  "Feb",
  "Mar",
  "Apr",
  "May",
  "Jun",
  "Jul",
  "Aug",
  "Sep",
  "Oct",
  "Nov",
  "Dec",
];

function monthDayParts(value: string): [string, number] {
  return [MONTH_NAMES[Number(value.slice(0, 2)) - 1]!, Number(value.slice(3))];
}

/** A window in the catalog's style: "Oct 20–Nov 2", or "Oct 15–31" within a month. */
export function holidayDatesLabel(dates: HolidayDates): string {
  const [startMonth, startDay] = monthDayParts(dates.start);
  const [endMonth, endDay] = monthDayParts(dates.end);
  return startMonth === endMonth && startDay <= endDay
    ? `${startMonth} ${startDay}–${endDay}`
    : `${startMonth} ${startDay}–${endMonth} ${endDay}`;
}

/** A row's own yearly window for a built-in holiday (the server's SeasonalDates). */
export interface HolidayDates {
  start: string;
  end: string;
}

/** The row's theme_dates, keyed by holiday. */
export function themeDatesOf(config: Config): Record<string, HolidayDates> {
  const out: Record<string, HolidayDates> = {};
  const raw = config.theme_dates;
  if (!raw || typeof raw !== "object") return out;
  for (const [key, entry] of Object.entries(raw as Record<string, unknown>)) {
    if (!entry || typeof entry !== "object") continue;
    const { start, end } = entry as Record<string, unknown>;
    out[key] = {
      start: typeof start === "string" ? start : "",
      end: typeof end === "string" ? end : "",
    };
  }
  return out;
}

/** Why the server would refuse a holiday's own dates, or null when they're complete. */
export function holidayDatesProblem(dates: HolidayDates): string | null {
  if (validMonthDay(dates.start) && validMonthDay(dates.end)) return null;
  return "Enter both dates as MM-DD, or clear them to use the usual dates.";
}

function isMDBListList(value: string): boolean {
  try {
    const raw = value.trim();
    const url = new URL(raw);
    const host = url.hostname.toLowerCase().replace(/\.$/, "");
    // The same rules as the server's ValidateMDBListURL. The server refuses
    // any userinfo, even an empty "https://@host", which URL reports as no
    // username, so look for "@" in the authority as typed.
    const authority = /^[a-z][a-z\d+.-]*:\/\/([^/?#]*)/i.exec(raw)?.[1] ?? "";
    return (
      (url.protocol === "https:" || url.protocol === "http:") &&
      !authority.includes("@") &&
      url.username === "" &&
      url.password === "" &&
      (url.port === "" || url.port === "80" || url.port === "443") &&
      (host === "mdblist.com" || host === "www.mdblist.com") &&
      url.pathname.startsWith("/lists/")
    );
  } catch {
    return false;
  }
}

/** Why the server would refuse this holiday, or null when it's complete. */
export function customHolidayProblem(holiday: CustomHoliday): string | null {
  if (holiday.name.trim() === "") return "Give the holiday a name.";
  if (!validMonthDay(holiday.start) || !validMonthDay(holiday.end)) {
    return "Enter the start and end as MM-DD, such as 07-04.";
  }
  if (!isMDBListList(holiday.list_url)) {
    return "Use a public MDBList list link (https://mdblist.com/lists/…).";
  }
  return null;
}
