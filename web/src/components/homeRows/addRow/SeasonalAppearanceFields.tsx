import { useRef, useState } from "react";
import { ChevronRight, ImageOff, ImagePlus, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useUploadSeasonalBackground } from "@/hooks/queries/seasonalBackground";
import {
  hasAppearance,
  prunedAppearance,
  seasonalBackgroundURL,
  type HolidayAppearance,
} from "@/lib/homeRows/seasonal";

/** What the server accepts, and what the button promises. */
const ACCEPTED_BACKGROUNDS = "image/jpeg,image/png,image/webp";

/**
 * One holiday's web appearance, folded away behind a small "Appearance"
 * disclosure so the row editor stays compact. It is the same block for a
 * built-in holiday and for one of the user's own.
 *
 * Everything here dresses the Seasonal Picks band on the web home page; the
 * apps keep showing the row as they always have, which the helper line says.
 */
export function SeasonalAppearanceFields({
  holiday,
  value,
  onChange,
}: {
  /** The holiday the appearance belongs to, for labels and messages. */
  holiday: string;
  value: HolidayAppearance;
  onChange: (next: HolidayAppearance | undefined) => void;
}) {
  // Open on a holiday that already has one, so an administrator sees what is
  // set without hunting for it.
  const [open, setOpen] = useState(() => hasAppearance(value));
  const [uploading, setUploading] = useState(false);
  const fileRef = useRef<HTMLInputElement>(null);
  const upload = useUploadSeasonalBackground();
  const set = (patch: Partial<HolidayAppearance>) =>
    onChange(prunedAppearance({ ...value, ...patch }));

  async function send(file: File) {
    setUploading(true);
    try {
      const uploaded = await upload.mutateAsync(file);
      set({ background: uploaded.ref });
    } catch {
      // The hook has already reported it; the row keeps the background it had.
    } finally {
      setUploading(false);
    }
  }

  return (
    <div className="border-border/60 grid gap-3 rounded-lg border px-3 py-2">
      <button
        type="button"
        aria-expanded={open}
        aria-label={`Appearance for ${holiday}`}
        onClick={() => setOpen((current) => !current)}
        className="hover:text-foreground flex items-center gap-2 text-left text-[13px] font-medium"
      >
        <ChevronRight
          aria-hidden
          className={`text-muted-foreground size-4 transition-transform ${open ? "rotate-90" : ""}`}
        />
        Appearance
        <span className="text-muted-foreground font-normal">
          {hasAppearance(value) ? "set" : "none"}
        </span>
      </button>
      {open ? (
        <div className="grid gap-3">
          <p className="text-muted-foreground text-xs">
            Shown on the web home page while this holiday is in season. The apps show a normal row.
          </p>

          <div className="flex items-center gap-2">
            <div className="border-border bg-muted/40 flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-md border">
              {value.background ? (
                <img
                  src={seasonalBackgroundURL(value.background)}
                  alt=""
                  className="size-full object-cover"
                />
              ) : (
                <ImageOff aria-hidden className="text-muted-foreground size-5" />
              )}
            </div>
            <div className="grid gap-1">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={uploading}
                onClick={() => fileRef.current?.click()}
              >
                <ImagePlus className="size-4" />
                {uploading ? "Uploading…" : value.background ? "Replace" : "Upload a background"}
              </Button>
              <span className="text-muted-foreground text-xs">JPEG, PNG or WebP.</span>
            </div>
            {value.background ? (
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`Remove ${holiday} background`}
                onClick={() => set({ background: undefined })}
              >
                <Trash2 className="size-4" />
              </Button>
            ) : null}
            <input
              ref={fileRef}
              type="file"
              accept={ACCEPTED_BACKGROUNDS}
              className="sr-only"
              aria-label={`Background image for ${holiday}`}
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                if (file) void send(file);
              }}
            />
          </div>
          <div className="grid gap-1">
            <Label htmlFor={`${holiday}-headline`}>Headline</Label>
            <Input
              id={`${holiday}-headline`}
              aria-label={`${holiday} headline`}
              placeholder="Come experience the magic"
              maxLength={120}
              className="h-8 text-[13px]"
              value={value.headline ?? ""}
              onChange={(event) => set({ headline: event.target.value })}
            />
          </div>
          <div className="grid gap-1">
            <Label htmlFor={`${holiday}-subtitle`}>Subtitle</Label>
            <Input
              id={`${holiday}-subtitle`}
              aria-label={`${holiday} subtitle`}
              placeholder="Only shows under the headline"
              maxLength={200}
              className="h-8 text-[13px]"
              value={value.subtitle ?? ""}
              onChange={(event) => set({ subtitle: event.target.value })}
            />
          </div>
          <div className="grid gap-1">
            <Label htmlFor={`${holiday}-button`}>Button label</Label>
            <Input
              id={`${holiday}-button`}
              aria-label={`${holiday} button label`}
              placeholder="Watch now"
              maxLength={40}
              className="h-8 text-[13px]"
              value={value.button_label ?? ""}
              onChange={(event) => set({ button_label: event.target.value })}
            />
            <p className="text-muted-foreground text-xs">
              Opens the row&apos;s own catalog page. Empty uses &quot;Watch now&quot;.
            </p>
          </div>
        </div>
      ) : null}
    </div>
  );
}
