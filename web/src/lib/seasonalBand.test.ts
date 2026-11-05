import { describe, expect, it } from "vitest";
import { isSeasonalWebRow, seasonalBandOf, SEASONAL_BAND_DEFAULT_BUTTON } from "./seasonalBand";

describe("seasonal band decisions", () => {
  it("needs a headline to draw a band", () => {
    expect(seasonalBandOf(undefined)).toBeNull();
    expect(seasonalBandOf({ headline: "   " })).toBeNull();
    expect(seasonalBandOf({})).toBeNull();
    expect(seasonalBandOf({ headline: "  Merry Christmas  " })).toEqual({
      backgroundURL: "",
      headline: "Merry Christmas",
      subtitle: "",
      buttonLabel: SEASONAL_BAND_DEFAULT_BUTTON,
    });
  });

  it("keeps its own button label when one is set", () => {
    expect(seasonalBandOf({ headline: "Merry", button_label: "See the picks" })?.buttonLabel).toBe(
      "See the picks",
    );
  });

  it("drops an unset label to the web's own default", () => {
    expect(seasonalBandOf({ headline: "Merry", button_label: "   " })?.buttonLabel).toBe(
      SEASONAL_BAND_DEFAULT_BUTTON,
    );
  });

  it("only treats a row as a seasonal web row when the server sent an appearance", () => {
    expect(isSeasonalWebRow(undefined)).toBe(false);
    expect(isSeasonalWebRow({})).toBe(false);
    expect(isSeasonalWebRow({ background_url: "/x.webp" })).toBe(true);
    expect(isSeasonalWebRow({ button_label: "Watch now" })).toBe(true);
  });
});
