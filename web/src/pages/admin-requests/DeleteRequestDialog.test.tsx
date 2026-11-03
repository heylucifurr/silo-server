import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { MediaRequest } from "@/api/types";
import type { AdminRequestDeleteOptions } from "@/api/v2/adminRequests";
import { DeleteRequestDialog } from "./DeleteRequestDialog";

const CREATED = "2026-01-01T00:00:00Z";
const request: MediaRequest = {
  id: "r1",
  provider: "tmdb",
  media_type: "movie",
  tmdb_id: 949,
  title: "Heat",
  status: "pending",
  outcome: "active",
  state: "pending",
  is_anime: false,
  seasons: [],
  season_progress: [],
  targets: [],
  created_at: CREATED,
  updated_at: CREATED,
};

function openDialog(onConfirm: (options: AdminRequestDeleteOptions) => void) {
  render(
    <DeleteRequestDialog request={request} open onOpenChange={() => {}} onConfirm={onConfirm} />,
  );
}

describe("DeleteRequestDialog", () => {
  afterEach(cleanup);

  it("asks the download server to drop the request by default", () => {
    const onConfirm = vi.fn();
    openDialog(onConfirm);

    fireEvent.click(screen.getByRole("button", { name: "Delete request" }));

    expect(onConfirm).toHaveBeenCalledWith({
      removeMedia: true,
      siloOnly: false,
    });
  });

  it("offers one download-server choice, not a separate files one", () => {
    openDialog(vi.fn());

    // Seerr deletes the files with the title and has no switch for keeping them,
    // so a second box would promise something the server cannot do.
    expect(screen.getAllByRole("checkbox")).toHaveLength(1);
    expect(screen.queryByRole("checkbox", { name: /delete the downloaded files/ })).toBeNull();
  });

  it("closes the Silo record only when the download-server box is cleared", () => {
    const onConfirm = vi.fn();
    openDialog(onConfirm);

    fireEvent.click(screen.getByRole("checkbox", { name: /remove it from the download server/ }));
    fireEvent.click(screen.getByRole("button", { name: "Delete request" }));

    expect(onConfirm).toHaveBeenCalledWith({
      removeMedia: false,
      siloOnly: true,
    });
  });
});
