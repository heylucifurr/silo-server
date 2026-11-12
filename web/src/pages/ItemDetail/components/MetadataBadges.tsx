import { useT } from "@/i18n";

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

  const t = useT();
  const advisoryLabel = advisorySource ? ADVISORY_SOURCE_LABELS[advisorySource] : undefined;
  const age = advisoryAge ?? 0;
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
            advisoryLabel
              ? t("web.detail.badges.advisorySourceTitle", { source: advisoryLabel, age })
              : t("web.detail.badges.advisoryTitle", { age })
          }
        >
          {advisoryLabel
            ? t("web.detail.badges.advisorySource", { source: advisoryLabel, age })
            : t("web.detail.badges.advisory", { age })}
        </span>
      )}
      {seasonCount != null && (
        <span className="metadata-badge">
          {t("web.detail.badges.seasons", { count: seasonCount, count_text: seasonCount })}
        </span>
      )}
      {seasonLabel && <span className="metadata-badge">{seasonLabel}</span>}
      {episodeCount != null && (
        <span className="metadata-badge">
          {t("web.detail.badges.episodes", {
            count: episodeCount,
            count_text: episodeCount,
          })}
        </span>
      )}
      {volumeCount != null && volumeCount > 0 && (
        <span className="metadata-badge">
          {t("web.detail.badges.volumes", { count: volumeCount, count_text: volumeCount })}
        </span>
      )}
      {chapterCount != null && chapterCount > 0 && (
        <span className="metadata-badge">
          {t("web.detail.badges.chapters", {
            count: chapterCount,
            count_text: chapterCount,
          })}
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
