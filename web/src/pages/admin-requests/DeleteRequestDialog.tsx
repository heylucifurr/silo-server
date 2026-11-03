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
 * Confirms deleting a request, and asks what else to remove: the request is
 * always closed in Silo (outcome `removed`); its download server entry and the
 * downloaded files are opt-in. Unchecking the download-server box is the escape
 * hatch for a server that cannot be reached, which leaves it holding the
 * request.
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
  const [deleteFiles, setDeleteFiles] = useState(false);

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
            onCheckedChange={(checked) => {
              const on = checked === true;
              setRemoveMedia(on);
              if (!on) setDeleteFiles(false);
            }}
          />
          <span>
            Also remove it from the download server
            <span className="text-muted-foreground block text-xs">
              Cancels its download. Leave this off when the server cannot be reached: Silo closes
              its own record and the server keeps the request.
            </span>
          </span>
        </label>
        <label htmlFor={`${id}-files`} className="flex items-start gap-2.5">
          <Checkbox
            id={`${id}-files`}
            checked={deleteFiles}
            disabled={!removeMedia}
            onCheckedChange={(checked) => setDeleteFiles(checked === true)}
          />
          <span>
            Also delete the downloaded files
            <span className="text-muted-foreground block text-xs">
              Only what this request downloaded. A copy already in the library is untouched.
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
                deleteFiles,
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
