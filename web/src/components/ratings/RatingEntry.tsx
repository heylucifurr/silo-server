// A separate file rather than an inlined data URI, so the logo stays out of
// the launch bundle.
import tmdbLogo from "@/assets/tmdb/tmdb-logo.svg?no-inline";
// IMDb's and Rotten Tomatoes' marks. See assets/ratings/README.md for why this
// fork draws artwork the upstream client does not.
import imdbLogo from "@/assets/ratings/imdb.svg?no-inline";
import rtAudienceLogo from "@/assets/ratings/rt-audience.svg?no-inline";
import rtCriticLogo from "@/assets/ratings/rt-critic.svg?no-inline";
import { cn } from "@/lib/utils";

import type { DisplayRating } from "./ratings";

/** The artwork drawn for each source that has any. */
const SOURCE_LOGOS: Record<string, string> = {
  imdb: imdbLogo,
  rt_critic: rtCriticLogo,
  rt_audience: rtAudienceLogo,
};

/**
 * The drawn height of a source's mark. TMDB's wordmark is wide and short, so it
 * sits lower than the square-ish marks beside it.
 */
function logoHeightClass(source: string, size: "sm" | "md"): string {
  if (source === "tmdb") return size === "sm" ? "h-2" : "h-2.5";
  return size === "sm" ? "h-3" : "h-4";
}

interface RatingEntryProps {
  rating: DisplayRating;
  size?: "sm" | "md";
  className?: string;
  /**
   * Draw the source as its logo rather than its text mark. Off by default, so
   * cards keep the plain-text marks; the detail hero turns it on.
   */
  sourceLogos?: boolean;
}

/**
 * A rating as its source's mark and its score: "IMDb 8.5", "RT 93%".
 *
 * TMDB's approved logo always stands in for its mark, as TMDB's terms allow.
 * The other marks are plain text unless the caller asks for artwork.
 */
export function RatingEntry({
  rating,
  size = "md",
  className,
  sourceLogos = false,
}: RatingEntryProps) {
  const sourceLogo = rating.source === "tmdb" ? tmdbLogo : SOURCE_LOGOS[rating.source];
  const logo = sourceLogos || rating.source === "tmdb" ? sourceLogo : undefined;

  return (
    <span className={cn("inline-flex items-center gap-1.5 whitespace-nowrap", className)}>
      {logo ? (
        <img
          src={logo}
          alt={rating.name}
          className={cn("w-auto shrink-0", logoHeightClass(rating.source, size))}
        />
      ) : (
        <span
          className={cn(
            "font-semibold tracking-tight opacity-75",
            size === "sm" ? "text-xs" : "text-[0.8125rem]",
          )}
        >
          {rating.name}
        </span>
      )}{" "}
      <span
        className={cn("font-bold tabular-nums", size === "sm" ? "text-sm" : "text-[0.9375rem]")}
      >
        {rating.display}
      </span>
    </span>
  );
}

