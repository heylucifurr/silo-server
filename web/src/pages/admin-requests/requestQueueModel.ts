import type { MediaRequest, RequestMediaType, RequestTarget } from "@/api/types";
import type { AdminRequestCounts, AdminRequestQueueView } from "@/api/v2/adminRequests";
import { t, type MessageKey } from "@/i18n";
import { canWithdrawRequest, formatRequestStatus } from "@/lib/mediaRequests";

export type { AdminRequestQueueView as RequestQueueView };

/** The queue's views, in tab order, with the count each one shows. `label` and `empty` are message ids. */
export const REQUEST_QUEUE_VIEWS: readonly {
  value: AdminRequestQueueView;
  label: MessageKey;
  empty: MessageKey;
}[] = [
  {
    value: "needs_approval",
    label: "web.admin.requests.queue.view.needsApproval",
    empty: "web.admin.requests.queue.view.needsApprovalEmpty",
  },
  {
    value: "in_progress",
    label: "web.admin.requests.queue.view.inProgress",
    empty: "web.admin.requests.queue.view.inProgressEmpty",
  },
  {
    value: "failed",
    label: "web.admin.requests.queue.view.failed",
    empty: "web.admin.requests.queue.view.failedEmpty",
  },
  {
    value: "done",
    label: "web.admin.requests.queue.view.done",
    empty: "web.admin.requests.queue.view.doneEmpty",
  },
];

export function parseRequestQueueView(value: string | null): AdminRequestQueueView | undefined {
  return REQUEST_QUEUE_VIEWS.find((view) => view.value === value)?.value;
}

/** The view the queue opens on: what needs approval, else what is under way. */
export function defaultRequestQueueView(counts: AdminRequestCounts): AdminRequestQueueView {
  return counts.needs_approval > 0 ? "needs_approval" : "in_progress";
}

/**
 * The view a request belongs to, by the server's own rule: needs approval
 * (pending), in progress (approved, queued or downloading), failed, and done
 * (completed, or closed by a decline or cancellation).
 */
export function requestQueueView(
  request: Pick<MediaRequest, "status" | "outcome">,
): AdminRequestQueueView {
  switch (request.outcome) {
    case "failed":
      return "failed";
    case "declined":
    case "cancelled":
    case "removed":
      return "done";
  }
  if (request.status === "pending") return "needs_approval";
  if (request.status === "completed") return "done";
  return "in_progress";
}

/**
 * What an admin can do with one request. Refresh is offered in a request's
 * detail dialog only: it re-reads the status from Seerr, which is worth doing
 * on the request under an admin's eye rather than on every row of the queue.
 */
export type RequestQueueAction = "approve" | "decline" | "retry" | "cancel" | "delete" | "refresh";

/**
 * What an admin can do with a request, by its view and the server's rules:
 * approve or decline what needs approval; cancel what is in progress while
 * nothing has been sent for it; retry or close (cancel) what failed. Delete is
 * offered everywhere else: it closes the Silo record and removes the request
 * from its download server, which is how an admin cleans up a request they no
 * longer want, including one that is already done. A request that was already
 * removed offers nothing: it is closed here and gone from its server.
 */
export function requestQueueActions(
  request: Pick<MediaRequest, "status" | "outcome" | "targets">,
): RequestQueueAction[] {
  if (request.outcome === "removed") return [];
  const withdrawable = canWithdrawRequest(request);
  switch (requestQueueView(request)) {
    case "needs_approval":
      return withdrawable ? ["approve", "decline", "delete"] : ["delete"];
    case "in_progress":
      return withdrawable ? ["cancel", "delete"] : ["delete"];
    case "failed":
      return ["retry", "cancel", "delete"];
    case "done":
      return ["delete"];
  }
}

/**
 * Whether "Refresh from Seerr" can do anything for a request: the server has to
 * support it, the request has to have a target that went somewhere with an id
 * Seerr knows, and it must be a request no person has closed, since the sync
 * leaves those as they are.
 */
export function canRefreshRequestFromSeerr(
  request: Pick<MediaRequest, "outcome" | "targets">,
  seerrSync: boolean | undefined,
): boolean {
  if (!seerrSync) return false;
  if (
    request.outcome === "declined" ||
    request.outcome === "cancelled" ||
    request.outcome === "removed"
  ) {
    return false;
  }
  return (request.targets ?? []).some((target) => Boolean(target.external_id));
}

/** Text for a WeFlix-only event, which upstream's message catalog does not have. */
interface EnglishText {
  english: string;
}

const EVENT_LABELS: Record<string, MessageKey | EnglishText> = {
  created: "web.admin.requests.event.created",
  approved: "web.admin.requests.event.approved",
  // The server records an admin's approval as a status change, and an
  // automatic one as `approved`.
  status_approved: "web.admin.requests.event.approved",
  status_queued: "web.admin.requests.event.queued",
  status_downloading: "web.admin.requests.event.downloading",
  status_completed: "web.admin.requests.event.downloaded",
  available_in_library: "web.admin.requests.event.inLibrary",
  outcome_declined: "web.admin.requests.event.declined",
  outcome_cancelled: "web.admin.requests.event.cancelled",
  outcome_failed: "web.admin.requests.event.failed",
  outcome_removed: { english: "Removed" },
  // The reconcile pass retires a request Seerr no longer has.
  removed_in_seerr: { english: "Removed in the download server" },
  // The status sync moved a target to match what Seerr reports.
  synced_from_seerr: { english: "Status synced" },
  retried: "web.admin.requests.event.retried",
  submit_deferred: "web.admin.requests.event.deferred",
};

/** A history entry's type in plain words; a type this client doesn't know shows as it is. */
export function formatRequestEventType(type: string): string {
  const label = Object.hasOwn(EVENT_LABELS, type) ? EVENT_LABELS[type] : undefined;
  if (!label) return type;
  return typeof label === "string" ? t(label) : label.english;
}

/** The account a `?user=` link names, or undefined for anything but a positive ID. */
export function parseRequestQueueUser(value: string | null): number | undefined {
  if (!value || !/^[1-9][0-9]{0,9}$/.test(value)) return undefined;
  const id = Number(value);
  // Account IDs are 4-byte integers on the server.
  return id <= 2_147_483_647 ? id : undefined;
}

export type RequestQueueMediaFilter = RequestMediaType | "all";

/** A target's status in words; its server's own status is shown beside it. */
export function formatTargetStatus(status: RequestTarget["status"]): string {
  return status === "failed" ? t("web.admin.requests.target.failed") : formatRequestStatus(status);
}

/** A target's server, falling back to the kind of integration. */
export function targetServerName(
  target: Pick<RequestTarget, "instance_name" | "integration_kind">,
) {
  return (
    target.instance_name || target.integration_kind || t("web.admin.requests.target.unknownServer")
  );
}

/** The request's own closing reason, or the error that stopped it. */
export function requestProblemText(
  request: Pick<MediaRequest, "outcome_reason" | "last_error">,
): { text: string; error: boolean } | null {
  if (request.last_error) return { text: request.last_error, error: true };
  if (request.outcome_reason) return { text: request.outcome_reason, error: false };
  return null;
}
