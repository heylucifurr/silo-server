interface MetadataBadgesProps {
  year?: string;
  contentRating?: string;
  /** Recommended minimum viewer age from an advisory service. */
  advisoryAge?: number;
  /** Who recommended advisoryAge, used to attribute the badge. */
  advisorySource?: string;
  duration?: string;
  seasonCount?: number;
  seasonLabel?: string;
  episodeCount?: number;
  volumeCount?: number;
  chapterCount?: number;
  status?: string;
}

const ADVISORY_SOURCE_LABELS: Record<string, string> = {
  commonsense: "Common Sense",
  mdblist: "MDBList",
};

export default function MetadataBadges({
  year,
  contentRating,
  advisoryAge,
  advisorySource,
  duration,
  seasonCount,
  seasonLabel,
  episodeCount,
  volumeCount,
  chapterCount,
  status,
}: MetadataBadgesProps) {
  // The artboard draws the year and the runtime as plain text beside the chips
  // rather than as chips of their own.
  const facts = [year, duration].filter((fact): fact is string => Boolean(fact));

  return (
    <div className="flex flex-wrap items-center gap-2">
      {facts.length > 0 && (
        <span className="text-foreground text-[0.9375rem] font-medium">
          {facts.join(" · ")}
        </span>
      )}
      {contentRating && <span className="metadata-badge">{contentRating}</span>}
      {advisoryAge != null && advisoryAge > 0 && (
        <span
          className="metadata-badge"
          title={
            advisorySource && ADVISORY_SOURCE_LABELS[advisorySource]
              ? `${ADVISORY_SOURCE_LABELS[advisorySource]} suggests age ${advisoryAge} and up.`
              : `Suggested for ages ${advisoryAge} and up.`
          }
        >
          {advisorySource && ADVISORY_SOURCE_LABELS[advisorySource]
            ? `${ADVISORY_SOURCE_LABELS[advisorySource]} ${advisoryAge}+`
            : `${advisoryAge}+`}
        </span>
      )}
      {seasonCount != null && (
        <span className="metadata-badge">
          {seasonCount} {seasonCount === 1 ? "Season" : "Seasons"}
        </span>
      )}
      {seasonLabel && <span className="metadata-badge">{seasonLabel}</span>}
      {episodeCount != null && (
        <span className="metadata-badge">
          {episodeCount} {episodeCount === 1 ? "Episode" : "Episodes"}
        </span>
      )}
      {volumeCount != null && volumeCount > 0 && (
        <span className="metadata-badge">
          {volumeCount} {volumeCount === 1 ? "Volume" : "Volumes"}
        </span>
      )}
      {chapterCount != null && chapterCount > 0 && (
        <span className="metadata-badge">
          {chapterCount} {chapterCount === 1 ? "Chapter" : "Chapters"}
        </span>
      )}
      {status && (
        <span className="metadata-badge border-primary/25 text-primary bg-primary/10">
          {status}
        </span>
      )}
    </div>
  );
}
