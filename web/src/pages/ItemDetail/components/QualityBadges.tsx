import type { SelectedMediaSummary } from "./selectedMediaSummary";

interface QualityBadgesProps {
  summary: SelectedMediaSummary;
}

export default function QualityBadges({ summary }: QualityBadgesProps) {
  const badges: string[] = [];
  if (summary.resolution) badges.push(summary.resolution);
  if (summary.videoRangeLabel) badges.push(summary.videoRangeLabel);
  if (summary.audioLabel) badges.push(summary.audioLabel);

  if (badges.length === 0) return null;

  return (
    <div className="flex flex-wrap items-center gap-2">
      {badges.map((label) => (
        // The pill the year, rating and runtime badges draw, in the accent: one
        // row of badges, one shape. Poster-card labels are another component
        // (lib/overlays) and keep their own treatment.
        <span key={label} className="metadata-badge border-primary/25 bg-primary/10 text-primary">
          {label}
        </span>
      ))}
    </div>
  );
}
