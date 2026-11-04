import { useId, useState } from "react";
import type { MediaRequest } from "@/api/types";
import type { AdminRequestDeleteOptions } from "@/api/v2/adminRequests";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Checkbox } from "@/components/ui/checkbox";

/**
 * Confirms deleting one request or a selected batch, and asks whether the
 * download server loses them too: every request is closed in Silo (outcome
 * `removed`), and each title is removed from Radarr or Sonarr unless the admin
 * asks for Silo only. There is one download-server choice, not two: Seerr's
 * remove call deletes the files with the title and offers no switch for keeping
 * them.
 */
export function DeleteRequestDialog({
  requests,
  open,
  onOpenChange,
  onConfirm,
  isPending,
}: {
  /** The requests being deleted; empty while the dialog is closed. */
  requests: readonly MediaRequest[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (options: AdminRequestDeleteOptions) => void;
  isPending?: boolean;
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      {/* Keyed by the selection and the open state, so every open starts from
          the defaults whatever the last one chose. */}
      <DeleteRequestContent
        key={`${requests.map((request) => request.id).join(",")}:${open ? "open" : "closed"}`}
        requests={requests}
        onConfirm={onConfirm}
        isPending={isPending}
      />
    </AlertDialog>
  );
}

function DeleteRequestContent({
  requests,
  onConfirm,
  isPending,
}: {
  requests: readonly MediaRequest[];
  onConfirm: (options: AdminRequestDeleteOptions) => void;
  isPending?: boolean;
}) {
  const id = useId();
  const [removeMedia, setRemoveMedia] = useState(true);
  const count = requests.length;
  const one = count === 1;
  // Seerr removes a whole series from Sonarr, not the seasons a request named,
  // so the copy has to say so: a season request would otherwise read as though
  // it only touched those seasons.
  const isSeries = requests.some((request) => request.media_type === "series");
  const serverNote = isSeries
    ? one
      ? "Removes the whole series from Sonarr and deletes its files, not just the seasons this request asked for."
      : "Removes each title from Radarr or Sonarr and deletes its files; a series goes whole, not just the seasons a request asked for."
    : one
      ? "Removes the movie from Radarr and deletes its files."
      : "Removes each movie from Radarr and deletes its files.";

  return (
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>
          {one ? "Delete this request?" : `Delete ${count} requests?`}
        </AlertDialogTitle>
        <AlertDialogDescription>
          {count === 0
            ? ""
            : one
              ? `"${requests[0]!.title}" is closed in Silo and marked removed. It stops showing as an open request.`
              : `${count} requests are closed in Silo and marked removed. They stop showing as open requests.`}
        </AlertDialogDescription>
      </AlertDialogHeader>
      <div className="grid gap-3 text-sm">
        <label htmlFor={`${id}-server`} className="flex items-start gap-2.5">
          <Checkbox
            id={`${id}-server`}
            checked={removeMedia}
            onCheckedChange={(checked) => setRemoveMedia(checked === true)}
          />
          <span>
            {one
              ? "Also remove it from the download server"
              : "Also remove them from the download server"}
            <span className="text-muted-foreground block text-xs">
              {serverNote} Leave this off when the server cannot be reached: Silo closes
              {one ? " its own record" : " its own records"} and the server keeps
              {one ? " the title" : " them"}.
            </span>
          </span>
        </label>
      </div>
      <AlertDialogFooter className="items-center sm:justify-between">
        <p className="text-muted-foreground text-sm">This can&apos;t be undone.</p>
        <div className="flex gap-2">
          <AlertDialogCancel>{one ? "Keep request" : "Keep requests"}</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={isPending}
            onClick={() =>
              onConfirm({
                removeMedia,
                siloOnly: !removeMedia,
              })
            }
          >
            {isPending ? (
              <>
                <span className="mr-2 inline-block h-3 w-3 animate-spin rounded-full border-2 border-current border-t-transparent" />
                {one ? "Delete request" : `Delete ${count} requests`}
              </>
            ) : one ? (
              "Delete request"
            ) : (
              `Delete ${count} requests`
            )}
          </AlertDialogAction>
        </div>
      </AlertDialogFooter>
    </AlertDialogContent>
  );
}
