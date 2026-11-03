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
      deleteFiles: false,
      siloOnly: false,
    });
  });

  it("passes the file deletion on when it is asked for", () => {
    const onConfirm = vi.fn();
    openDialog(onConfirm);

    fireEvent.click(screen.getByRole("checkbox", { name: /delete the downloaded files/ }));
    fireEvent.click(screen.getByRole("button", { name: "Delete request" }));

    expect(onConfirm).toHaveBeenCalledWith({
      removeMedia: true,
      deleteFiles: true,
      siloOnly: false,
    });
  });

  it("closes the Silo record only when the download-server box is cleared", () => {
    const onConfirm = vi.fn();
    openDialog(onConfirm);

    fireEvent.click(screen.getByRole("checkbox", { name: /remove it from the download server/ }));
    // The files box follows the server box: with the server left alone there
    // is nothing for Silo to delete.
    expect(screen.getByRole("checkbox", { name: /delete the downloaded files/ })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Delete request" }));

    expect(onConfirm).toHaveBeenCalledWith({
      removeMedia: false,
      deleteFiles: false,
      siloOnly: true,
    });
  });
});
