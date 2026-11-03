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
 * Confirms deleting a request, and asks whether the download server loses it
 * too: the request is always closed in Silo (outcome `removed`), and the title
 * is removed from Radarr or Sonarr unless the admin asks for Silo only. There is
 * one download-server choice, not two: Seerr's remove call deletes the files
 * with the title and offers no switch for keeping them.
 */
export function DeleteRequestDialog({
  request,
  open,
  onOpenChange,
  onConfirm,
  isPending,
}: {
  request: MediaRequest | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (options: AdminRequestDeleteOptions) => void;
  isPending?: boolean;
}) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      {/* Keyed by the request and the open state, so every open starts from the
          defaults whatever the last one chose. */}
      <DeleteRequestContent
        key={`${request?.id ?? "none"}:${open ? "open" : "closed"}`}
        request={request}
        onConfirm={onConfirm}
        isPending={isPending}
      />
    </AlertDialog>
  );
}

function DeleteRequestContent({
  request,
  onConfirm,
  isPending,
}: {
  request: MediaRequest | null;
  onConfirm: (options: AdminRequestDeleteOptions) => void;
  isPending?: boolean;
}) {
  const id = useId();
  const [removeMedia, setRemoveMedia] = useState(true);

  return (
    <AlertDialogContent>
      <AlertDialogHeader>
        <AlertDialogTitle>Delete this request?</AlertDialogTitle>
        <AlertDialogDescription>
          {request
            ? `"${request.title}" is closed in Silo and marked removed. It stops showing as an open request.`
            : ""}
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
            Also remove it from the download server
            <span className="text-muted-foreground block text-xs">
              Removes the title from Radarr or Sonarr and deletes its files. Leave this off when the
              server cannot be reached: Silo closes its own record and the server keeps the title.
            </span>
          </span>
        </label>
      </div>
      <AlertDialogFooter className="items-center sm:justify-between">
        <p className="text-muted-foreground text-sm">This can&apos;t be undone.</p>
        <div className="flex gap-2">
          <AlertDialogCancel>Keep request</AlertDialogCancel>
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
                Delete request
              </>
            ) : (
              "Delete request"
            )}
          </AlertDialogAction>
        </div>
      </AlertDialogFooter>
    </AlertDialogContent>
  );
}
