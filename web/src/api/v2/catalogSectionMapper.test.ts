import { describe, expect, it } from "vitest";

import { sectionFromV2 } from "@/api/v2/catalog";

// sectionFromV2 maps every card through catalogItemFromV2, which builds each card
// field by field. A field the server sends but that mapper does not copy is
// dropped silently: the API returns the value, the UI renders nothing, and only
// the type checker could have noticed — which is how the curated "Most
// Anticipated" hero shipped showing a Play button for an unreleased title and
// linking More Info at /item/ext:... instead of the title page.
//
// The hero decides Coming soon versus Play from `availability`, and builds its
// title link from `tmdb_id`, so both are pinned on the way through the mapper.
describe("sectionFromV2 anticipated card fields", () => {
  function sectionWith(card: Record<string, unknown>) {
    return sectionFromV2({
      id: "sec-1",
      section_type: "anticipated",
      title: "Most Anticipated",
      featured: true,
      item_limit: 20,
      total_count: 1,
      is_custom: false,
      customized: false,
      items: [
        {
          content_id: "ext:movie-1003596",
          type: "movie",
          title: "Avengers: Doomsday",
          year: 2026,
          genres: [],
          keywords: [],
          overview: "",
          poster_url: "",
          backdrop_url: "",
          logo_url: "",
          ...card,
        },
      ],
    } as unknown as Parameters<typeof sectionFromV2>[0]);
  }

  it("keeps the availability, release date and TMDB id of an unowned slide", () => {
    const section = sectionWith({
      availability: "unreleased",
      release_date: "2026-12-16",
      tmdb_id: 1003596,
    });

    expect(section.items[0]?.availability).toBe("unreleased");
    expect(section.items[0]?.release_date).toBe("2026-12-16");
    expect(section.items[0]?.tmdb_id).toBe(1003596);
  });

  it("leaves an ordinary card without an availability", () => {
    const section = sectionWith({});

    expect(section.items[0]?.availability).toBeUndefined();
    expect(section.items[0]?.tmdb_id).toBeUndefined();
  });
});
