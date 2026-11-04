import { Ban, Check, Loader2, RefreshCw, RotateCw, Trash2, X } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { MediaRequest } from "@/api/types";
import { Button } from "@/components/ui/button";
import { requestQueueActions, type RequestQueueAction } from "./requestQueueModel";

/** Runs a queue action; decline and cancel ask for an optional reason first. */
export interface RequestQueueActionHandlers {
  run: (action: RequestQueueAction, request: MediaRequest) => void;
  /** The action for this request that is on its way to the server, if any. */
  busyAction: (id: string) => RequestQueueAction | undefined;
  /** Every action waits, as while a bulk action runs. */
  locked: boolean;
}

const ACTIONS: Record<
  RequestQueueAction,
  { label: string; icon: LucideIcon; variant: "default" | "outline"; className?: string }
> = {
  approve: { label: "Approve", icon: Check, variant: "default" },
  decline: { label: "Decline", icon: X, variant: "outline" },
  retry: { label: "Retry", icon: RefreshCw, variant: "default" },
  cancel: { label: "Cancel request", icon: Ban, variant: "outline" },
  delete: {
    label: "Delete request",
    icon: Trash2,
    variant: "outline",
    className: "text-destructive hover:text-destructive",
  },
  // Offered in the request's detail dialog, not on the queue's rows.
  refresh: { label: "Refresh from Seerr", icon: RotateCw, variant: "outline" },
};

/**
 * One action's button: its label, its icon, and the spinner the action shows
 * while the server answers. Every action on a request waits while one of them
 * is running.
 */
export function RequestActionButton({
  action,
  request,
  handlers,
}: {
  action: RequestQueueAction;
  request: MediaRequest;
  handlers: RequestQueueActionHandlers;
}) {
  const { label, icon: Icon, variant, className } = ACTIONS[action];
  const running = handlers.busyAction(request.id);
  const isRunning = action === running;
  return (
    <Button
      size="sm"
      variant={variant}
      className={className}
      disabled={handlers.locked || running !== undefined}
      aria-label={`${label}: ${request.title}`}
      aria-busy={isRunning || undefined}
      onClick={() => handlers.run(action, request)}
    >
      {isRunning ? (
        <Loader2 aria-hidden="true" className="animate-spin" />
      ) : (
        <Icon aria-hidden="true" />
      )}
      {label}
    </Button>
  );
}

/** The actions a request allows in the queue; none for one that is done. */
export function RequestActionButtons({
  request,
  handlers,
}: {
  request: MediaRequest;
  handlers: RequestQueueActionHandlers;
}) {
  const actions = requestQueueActions(request);
  if (actions.length === 0) return null;
  return (
    <div className="flex flex-wrap gap-2">
      {actions.map((action) => (
        <RequestActionButton key={action} action={action} request={request} handlers={handlers} />
      ))}
    </div>
  );
}
