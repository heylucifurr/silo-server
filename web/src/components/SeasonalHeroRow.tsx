import { Link } from "react-router";
import SectionRow from "@/components/SectionRow";
import { useMediaQuery } from "@/hooks/useMediaQuery";
import { useUICustomization } from "@/hooks/useUICustomization";
import { buildSectionCatalogHref } from "@/pages/catalogSearchParams";
import {
  seasonalBandOf,
  SEASONAL_BAND_INFO_CARD_QUERY,
  type SeasonalBand,
} from "@/lib/seasonalBand";
import { seasonalInfoCardWidthClasses, type PosterSize } from "@/lib/uiCustomization";
import type { ResolvedSection } from "@/api/types";
import type { components } from "@/api/v2/schema";

type SectionAppearance = components["schemas"]["SectionAppearance"];

/**
 * The overlay over the band's artwork: the left third darkens under the text,
 * and the top and bottom edges melt into the rows above and below. Three
 * gradients, in the order Prime Video paints them, with their navy swapped for
 * this app's page colour so the fades land exactly on the page.
 */
const BAND_OVERLAY =
  "linear-gradient(270deg, rgba(0,0,0,0), rgba(0,0,0,.8) 86%)," +
  "linear-gradient(0deg, rgba(0,0,0,0) 72%, var(--background))," +
  "linear-gradient(180deg, rgba(0,0,0,0) 72%, var(--background))";

/**
 * The band's text as the row's first card: centred in a box two Featured
 * posters wide, so the row starts with words and the posters continue off the
 * right edge. Prime Video's measurements, in rem so the browser's text-size
 * setting still scales them.
 */
function SeasonalInfoCard({
  band,
  href,
  size,
}: {
  band: SeasonalBand;
  href: string;
  size: PosterSize;
}) {
  return (
    <div
      className={`${seasonalInfoCardWidthClasses(size)} flex h-full flex-col items-center justify-center px-1 py-[1.125rem] text-center`}
    >
      <h2 className="font-display line-clamp-2 max-w-[31.25rem] text-[2.5rem] leading-[2.75rem] font-bold tracking-tight text-balance text-white lg:text-[3.125rem] lg:leading-[3.5rem]">
        {band.headline}
      </h2>
      {band.subtitle ? (
        <span className="mb-[1.125rem] text-[1.25rem] leading-[1.4375rem] font-medium text-white/85">
          {band.subtitle}
        </span>
      ) : null}
      <Link
        to={href}
        className="rounded-[0.5rem] bg-white/20 px-[1.3125rem] py-2 text-[1.25rem] leading-[1.4375rem] font-medium text-white transition-colors hover:bg-white/30 focus-visible:ring-2 focus-visible:ring-white/60 focus-visible:outline-none"
      >
        {band.buttonLabel}
      </Link>
    </div>
  );
}

/**
 * The band's text stacked above the row, for the screens that do not draw the
 * info card inside it.
 */
function SeasonalBandHeading({ band, href }: { band: SeasonalBand; href: string }) {
  return (
    <div className="mb-5 grid justify-items-start gap-3 px-4 sm:px-6">
      <h2 className="font-display line-clamp-2 max-w-[31.25rem] text-[2rem] leading-[2.25rem] font-bold tracking-tight text-balance text-white sm:text-[2.5rem] sm:leading-[2.75rem]">
        {band.headline}
      </h2>
      {band.subtitle ? (
        <p className="text-[1.125rem] leading-[1.3125rem] font-medium text-white/85">
          {band.subtitle}
        </p>
      ) : null}
      <Link
        to={href}
        className="rounded-[0.5rem] bg-white/20 px-[1.3125rem] py-2 text-[1.125rem] leading-[1.3125rem] font-medium text-white transition-colors hover:bg-white/30 focus-visible:ring-2 focus-visible:ring-white/60 focus-visible:outline-none"
      >
        {band.buttonLabel}
      </Link>
    </div>
  );
}

/**
 * A Seasonal Picks row as the web home page's hero band: the artwork and the
 * overlay behind, the row's own carousel in front of them, vertically centred.
 *
 * On a wide screen the band's text is the row's first card, so it scrolls away
 * with the posters exactly as Prime Video's does; on a narrow one the card
 * would leave room for barely one poster, so the text stacks above the row
 * instead and the artwork stays behind both. The row itself is SectionRow's,
 * asked for the Featured presentation, without a header, with the info card as
 * its lead slide: no second card component, and no second carousel.
 */
export default function SeasonalHeroRow({
  section,
  appearance,
  priority = false,
  onNearEnd,
}: {
  section: ResolvedSection;
  appearance: SectionAppearance | undefined;
  priority?: boolean;
  onNearEnd?: (sectionId: string) => void;
}) {
  const band = seasonalBandOf(appearance);
  // Read before any branch: hooks may not sit behind a return.
  const infoCard = useMediaQuery(SEASONAL_BAND_INFO_CARD_QUERY, true);
  const { cardPresentation } = useUICustomization();

  if (!band) {
    // No headline: the row keeps the Featured card size and nothing else moves.
    return (
      <SectionRow
        section={section}
        presentation="featured"
        priority={priority}
        onNearEnd={onNearEnd}
      />
    );
  }

  const href = buildSectionCatalogHref({
    scope: "home",
    sectionId: section.id,
    title: section.title,
  });
  // Without an uploaded background the band borrows the first title's backdrop,
  // blurred and dimmed, so it still reads as a picture rather than a flat panel.
  const uploaded = band.backgroundURL;
  const backdrop = uploaded || (section.items[0]?.backdrop_url ?? "");

  return (
    <section aria-label={section.title} className="relative isolate">
      {/* The artwork and the overlay do not scroll: the row travels over them. */}
      <div aria-hidden className="absolute inset-0 overflow-hidden">
        {backdrop ? (
          <img
            src={backdrop}
            alt=""
            className={
              uploaded
                ? "h-full w-full object-cover object-center"
                : "h-full w-full scale-110 object-cover object-center opacity-50 blur-xl"
            }
          />
        ) : null}
        <div className="absolute inset-0" style={{ background: BAND_OVERLAY }} />
      </div>

      {/* The band's height: the row's own, plus the artwork showing ~95px above
          and below it on a desktop screen. */}
      <div className="relative py-10 sm:py-14 lg:py-24">
        {infoCard ? null : <SeasonalBandHeading band={band} href={href} />}
        <SectionRow
          section={section}
          presentation="featured"
          hideHeader
          priority={priority}
          onNearEnd={onNearEnd}
          lead={
            infoCard ? (
              <SeasonalInfoCard band={band} href={href} size={cardPresentation.poster_size} />
            ) : undefined
          }
        />
      </div>
    </section>
  );
}
