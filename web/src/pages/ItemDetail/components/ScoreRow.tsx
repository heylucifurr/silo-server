import { RatingEntry } from "@/components/ratings/RatingEntry";
import { ratingSourceAccent, type DisplayRating } from "@/components/ratings/ratings";

interface ScoreRowProps {
  /** The ratings the server chose for this title, in display order. */
  ratings?: readonly DisplayRating[] | null;
  /** How many TMDB votes a request title's TMDB score rests on. */
  tmdbVoteCount?: number | null;
}

const voteCountFormat = new Intl.NumberFormat(undefined, {
  notation: "compact",
  maximumFractionDigits: 1,
});

export default function ScoreRow({ ratings, tmdbVoteCount }: ScoreRowProps) {
  if (!ratings?.length) return null;

  return (
    <div className="text-primary flex flex-wrap items-center gap-x-5 gap-y-2">
      {ratings.map((rating) => (
        <span
          key={rating.source}
          // A mark Silo draws as text carries its source's colour, so the row
          // reads as sources rather than as scores. TMDB's mark is its approved
          // logo, which already carries the colour, so its score keeps the
          // row's own.
          style={
            rating.source === "tmdb" ? undefined : { color: ratingSourceAccent(rating.source) }
          }
          className="inline-flex items-center gap-1.5"
        >
          <RatingEntry rating={rating} />
          {rating.source === "tmdb" && tmdbVoteCount ? (
            <span className="text-muted-foreground text-xs tabular-nums">
              {voteCountFormat.format(tmdbVoteCount)} votes
            </span>
          ) : null}
        </span>
      ))}
    </div>
  );
}
