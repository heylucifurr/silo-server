/**
 * What the web home page makes of a Seasonal Picks row's appearance: whether
 * the row draws as the hero band, and with what.
 *
 * Data and decisions only, so the parts worth pinning down are testable without
 * a browser. SeasonalHeroRow draws them; nothing else reads either.
 */

import type { components } from "@/api/v2/schema";

type SectionAppearance = components["schemas"]["SectionAppearance"];

/**
 * The band's text and background, once a row is known to draw one. The
 * background may be empty: the band then borrows the first card's backdrop.
 */
export interface SeasonalBand {
  /** Uploaded background, or "" to fall back to the first card's backdrop. */
  backgroundURL: string;
  headline: string;
  subtitle: string;
  /** Never empty: the server may send no label, the web owns the default. */
  buttonLabel: string;
}

/** The label the band's button falls back to. */
export const SEASONAL_BAND_DEFAULT_BUTTON = "Watch now";

/**
 * The viewport width from which the band draws its text as a card inside the
 * row, and below which it stacks the same text above the row. lg, the step the
 * Featured cards grow at: the info card is two of them wide, so it needs the
 * room a smaller screen gives to the posters themselves.
 */
export const SEASONAL_BAND_INFO_CARD_QUERY = "(min-width: 64rem)";

/**
 * A headline is what makes the row a band: it is the line the whole layout is
 * built around. Without one the row keeps the Featured row's card size — a
 * background on its own still reads as a showcase — and null is the plain
 * poster row every other seasonal row draws as.
 */
export function seasonalBandOf(appearance: SectionAppearance | undefined): SeasonalBand | null {
  const headline = appearance?.headline?.trim() ?? "";
  if (headline === "") return null;
  return {
    backgroundURL: appearance?.background_url?.trim() ?? "",
    headline,
    subtitle: appearance?.subtitle?.trim() ?? "",
    buttonLabel: appearance?.button_label?.trim() || SEASONAL_BAND_DEFAULT_BUTTON,
  };
}

/**
 * True when the web should draw this row with the Featured row's card size,
 * band or not. The server sends an appearance only for a seasonal row with a
 * holiday in season, so nothing else on the page is affected.
 */
export function isSeasonalWebRow(appearance: SectionAppearance | undefined): boolean {
  if (!appearance) return false;
  return Boolean(
    appearance.background_url?.trim() ||
    appearance.headline?.trim() ||
    appearance.subtitle?.trim() ||
    appearance.button_label?.trim(),
  );
}
