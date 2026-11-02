import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { fetchAnticipated, saveAnticipated, searchAnticipated } from "@/api/v2/anticipated";
import { V2ProblemError } from "@/api/v2/request";
import AdminAnticipated from "./AdminAnticipated";

vi.mock("@/api/v2/anticipated", () => ({
  fetchAnticipated: vi.fn(),
  saveAnticipated: vi.fn(),
  searchAnticipated: vi.fn(),
}));

const fetchMock = vi.mocked(fetchAnticipated);
const saveMock = vi.mocked(saveAnticipated);
const searchMock = vi.mocked(searchAnticipated);

function entry(overrides: Record<string, unknown> = {}) {
  return {
    media_type: "movie" as const,
    tmdb_id: 550,
    position: 0,
    title: "Fight Club",
    state: "active" as const,
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  fetchMock.mockResolvedValue({ list: { entries: [entry()], max: 50 }, etag: '"one"' });
});

describe("AdminAnticipated", () => {
  it("loads the curated list", async () => {
    render(<AdminAnticipated />);
    expect(await screen.findByText("Fight Club")).toBeVisible();
  });

  it("adds a search result and saves the whole list under the ETag it read", async () => {
    searchMock.mockResolvedValue([
      { media_type: "series", tmdb_id: 1399, title: "Game of Thrones", year: 2011 },
    ]);
    saveMock.mockResolvedValue({
      list: {
        entries: [
          entry(),
          entry({ media_type: "series", tmdb_id: 1399, position: 1, title: "Game of Thrones" }),
        ],
        max: 50,
      },
      etag: '"two"',
    });
    const user = userEvent.setup();
    render(<AdminAnticipated />);
    await screen.findByText("Fight Club");

    await user.type(screen.getByLabelText("Search TMDB"), "Thrones");
    await user.click(screen.getByRole("button", { name: "Search" }));
    await user.click(await screen.findByRole("button", { name: "Add" }));

    expect(await screen.findByText("Game of Thrones")).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(saveMock).toHaveBeenCalledTimes(1));
    expect(saveMock).toHaveBeenCalledWith(
      [
        { media_type: "movie", tmdb_id: 550 },
        { media_type: "series", tmdb_id: 1399 },
      ],
      '"one"',
    );
  });

  it("removes an entry", async () => {
    const user = userEvent.setup();
    render(<AdminAnticipated />);
    await screen.findByText("Fight Club");

    await user.click(screen.getByRole("button", { name: /Remove Fight Club/ }));
    expect(screen.getByText("Nothing yet. Search above to add a title.")).toBeVisible();
  });

  it("reports a conflict rather than clobbering another admin's save", async () => {
    saveMock.mockRejectedValue(
      Object.assign(Object.create(V2ProblemError.prototype), {
        status: 412,
        problem: { status: 412 },
        message: "stale",
      }),
    );
    const user = userEvent.setup();
    render(<AdminAnticipated />);
    await screen.findByText("Fight Club");

    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText(/Someone else changed this list/)).toBeVisible();
  });
});
