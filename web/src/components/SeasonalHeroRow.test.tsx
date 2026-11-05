import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ResolvedSection, SectionItem } from "@/api/types";
import type { components } from "@/api/v2/schema";
import SeasonalHeroRow from "./SeasonalHeroRow";

type Appearance = components["schemas"]["SectionAppearance"];

/**
 * The row is another component's, and tested there. What matters here is that
 * the band asks for the Featured card treatment, drops the row header, and
 * hands its text to the row as the lead slide â€” so the tests play the info card
 * where the carousel would put it.
 */
let latestRow: { presentation?: string; hideHeader?: boolean; lead?: ReactNode } | undefined;
vi.mock("@/components/SectionRow", () => ({
  default: (props: { presentation?: string; hideHeader?: boolean; lead?: ReactNode }) => {
    latestRow = {
      presentation: props.presentation,
      hideHeader: props.hideHeader,
      lead: props.lead,
    };
    return <div data-testid="cards">{props.lead}</div>;
  },
}));

function movie(contentId: string, backdrop: string): SectionItem {
  return {
    content_id: contentId,
    type: "movie",
    title: contentId,
    year: 2001,
    genres: [],
    status: "matched",
    rating_imdb: null,
    overview: "",
    poster_url: "",
    poster_thumbhash: "",
    backdrop_url: backdrop,
    backdrop_thumbhash: "",
    logo_url: "",
  };
}

function seasonalSection(items: SectionItem[]): ResolvedSection {
  return {
    id: "seasonal",
    section_type: "seasonal_themed",
    title: "Christmas Favorites",
    featured: false,
    item_limit: 20,
    total_count: items.length,
    is_custom: false,
    customized: false,
    items,
  };
}

/** jsdom has no matchMedia: the band reads that as a wide screen unless a test
 * says otherwise, which is the case a phone would be. */
function stubNarrowScreen() {
  window.matchMedia = ((query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })) as unknown as typeof window.matchMedia;
}

function renderBand(section: ResolvedSection, appearance: Appearance | undefined) {
  return renderToStaticMarkup(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <SeasonalHeroRow section={section} appearance={appearance} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const CHRISTMAS: Appearance = {
  background_url: "/api/v2/seasonal-backgrounds/0123456789abcdef.webp",
  headline: "Come experience the magic",
  subtitle: "Your Hogwarts Adventure Awaits",
  button_label: "Watch now",
};

describe("SeasonalHeroRow", () => {
  beforeEach(() => {
    latestRow = undefined;
    // Restore the jsdom default (no matchMedia) between tests.
    Reflect.deleteProperty(window, "matchMedia");
  });

  afterEach(() => {
    cleanup();
    Reflect.deleteProperty(window, "matchMedia");
  });

  it("hands the row its text as the lead slide, at the Featured treatment", () => {
    const markup = renderBand(seasonalSection([movie("movie-1", "/backdrop-1.jpg")]), CHRISTMAS);

    expect(markup).toContain("Come experience the magic");
    expect(markup).toContain("Your Hogwarts Adventure Awaits");
    // The button opens the row's own catalog page, as "Explore all" does.
    expect(markup).toContain(
      'href="/catalog?source=section&amp;scope=home&amp;section_id=seasonal&amp;title=Christmas+Favorites"',
    );
    // The info card is two Featured posters wide and rides the row's list.
    expect(markup).toContain("w-[296px]");
    expect(latestRow?.presentation).toBe("featured");
    expect(latestRow?.hideHeader).toBe(true);
    expect(latestRow?.lead).toBeTruthy();
  });

  it("draws the uploaded background behind the row, under the overlay", () => {
    const markup = renderBand(seasonalSection([movie("movie-1", "/backdrop-1.jpg")]), CHRISTMAS);

    expect(markup).toContain("/api/v2/seasonal-backgrounds/0123456789abcdef.webp");
    // The backdrop the band was given wins over the first card's.
    expect(markup).not.toContain("/backdrop-1.jpg");
    // The overlay is painted over the artwork, not scrolled with the row.
    expect(markup).toContain("linear-gradient(270deg");
    expect(markup).toContain("var(--background)");
  });

  it("borrows the first card's backdrop, blurred, when no background was uploaded", () => {
    const markup = renderBand(seasonalSection([movie("movie-1", "/backdrop-1.jpg")]), {
      headline: "Spooky season",
    });

    expect(markup).toContain("/backdrop-1.jpg");
    expect(markup).toContain("blur-xl");
    // No button label was set, so the web's own default stands.
    expect(markup).toContain("Watch now");
  });

  it("stacks the text above the row instead of inside it on a narrow screen", () => {
    // A client render: server rendering always takes the wide-screen fallback,
    // which is why the other cases can be static markup.
    stubNarrowScreen();
    const { container } = render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <SeasonalHeroRow
            section={seasonalSection([movie("movie-1", "/backdrop-1.jpg")])}
            appearance={CHRISTMAS}
          />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(screen.getByText("Come experience the magic")).toBeTruthy();
    // No info card in the row: the text is the block above it.
    expect(container.innerHTML).not.toContain("w-[296px]");
    expect(latestRow?.lead).toBeUndefined();
    expect(latestRow?.presentation).toBe("featured");
  });

  it("draws a plain Featured-size row when the row has no headline", () => {
    const markup = renderBand(seasonalSection([movie("movie-1", "/backdrop-1.jpg")]), {
      background_url: "/api/v2/seasonal-backgrounds/0123456789abcdef.webp",
    });

    expect(markup).not.toContain("<h2");
    expect(markup).not.toContain("linear-gradient");
    expect(latestRow).toEqual({
      presentation: "featured",
      hideHeader: undefined,
      lead: undefined,
    });
  });
});
