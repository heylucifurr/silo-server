// @vitest-environment node

import { describe, expect, it } from "vitest";
import type { MediaRequest } from "@/api/types";
import {
  defaultRequestQueueView,
  parseRequestQueueUser,
  parseRequestQueueView,
  requestQueueActions,
  requestQueueView,
} from "./requestQueueModel";

type Fields = Pick<MediaRequest, "status" | "outcome" | "targets">;
const sent = [{ quality: "1080p", status: "queued" }] as MediaRequest["targets"];

describe("requestQueueView", () => {
  it.each<[string, Fields, string]>([
    ["pending", { status: "pending", outcome: "active" }, "needs_approval"],
    ["approved", { status: "approved", outcome: "active" }, "in_progress"],
    ["queued", { status: "queued", outcome: "active" }, "in_progress"],
    ["downloading", { status: "downloading", outcome: "active" }, "in_progress"],
    ["completed", { status: "completed", outcome: "active" }, "done"],
    ["failed", { status: "approved", outcome: "failed" }, "failed"],
    ["declined while pending", { status: "pending", outcome: "declined" }, "done"],
    ["cancelled", { status: "approved", outcome: "cancelled" }, "done"],
    ["removed", { status: "approved", outcome: "removed" }, "done"],
  ])("puts a %s request where the server does", (_label, request, view) => {
    expect(requestQueueView(request)).toBe(view);
  });
});

describe("requestQueueActions", () => {
  it.each<[string, Fields, string[]]>([
    ["pending", { status: "pending", outcome: "active" }, ["approve", "decline", "delete"]],
    [
      "approved, nothing sent",
      { status: "approved", outcome: "active", targets: [] },
      ["cancel", "delete"],
    ],
    // Nothing left to send for it, but an admin can still delete it: the
    // delete closes the Silo record and removes the request from its server.
    ["approved and sent", { status: "approved", outcome: "active", targets: sent }, ["delete"]],
    ["queued", { status: "queued", outcome: "active", targets: sent }, ["delete"]],
    // Admin cancel accepts only an active request, so a failed one offers Retry alone.
    [
      "failed",
      { status: "approved", outcome: "failed", targets: [] },
      ["retry", "cancel", "delete"],
    ],
    ["completed", { status: "completed", outcome: "active" }, ["delete"]],
    ["declined", { status: "pending", outcome: "declined" }, ["delete"]],
    // Already removed: closed here and gone from its server, so nothing to do.
    ["removed", { status: "approved", outcome: "removed" }, []],
  ])("offers a %s request only what the server allows", (_label, request, actions) => {
    expect(requestQueueActions(request)).toEqual(actions);
  });
});

describe("queue links", () => {
  it("opens on what needs approval, else on what is under way", () => {
    const counts = { needs_approval: 0, in_progress: 0, failed: 2, done: 9 };
    expect(defaultRequestQueueView(counts)).toBe("in_progress");
    expect(defaultRequestQueueView({ ...counts, needs_approval: 1 })).toBe("needs_approval");
  });

  it("reads only known views and positive account IDs", () => {
    expect(parseRequestQueueView("failed")).toBe("failed");
    expect(parseRequestQueueView("settings")).toBeUndefined();
    expect(parseRequestQueueView(null)).toBeUndefined();
    expect(parseRequestQueueUser("7")).toBe(7);
    for (const value of ["0", "-1", "7a", "01", "", null, "99999999999"]) {
      expect(parseRequestQueueUser(value)).toBeUndefined();
    }
  });
});
