import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RequestMediaDetail } from "@/api/types";
import type { WatchlistTitleEntry } from "@/api/v2/watchlistTitles";
import { requestKeys } from "@/hooks/queries/keys";
import RequestActionBar from "./RequestActionBar";

const mocks = vi.hoisted(() => ({
  featureStatus: vi.fn(),
  createRequest: vi.fn(),
  addWatchlistTitle: vi.fn(),
  deleteWatchlistTitle: vi.fn(),
  toastSuccess: vi.fn(),
  toastError: vi.fn(),
}));

vi.mock("@/hooks/queries/useRequests", async () => {
  const actual = await vi.importActual<typeof import("@/hooks/queries/useRequests")>(
    "@/hooks/queries/useRequests",
  );
  const idle = { mutate: vi.fn(), isPending: false };
  return {
    ...actual,
    useRequestFeatureStatus: () => mocks.featureStatus(),
    useCreateMediaRequest: () => mocks.createRequest(),
    useCancelMediaRequest: () => idle,
    useToggleRequestFollow: () => idle,
    useMediaRequest: () => ({ data: undefined }),
    useMyMediaRequests: () => ({ data: undefined }),
  };
});
vi.mock("@/api/v2/watchlistTitles", () => ({
  addWatchlistTitleV2: (...args: unknown[]) => mocks.addWatchlistTitle(...args),
  deleteWatchlistTitleV2: (...args: unknown[]) => mocks.deleteWatchlistTitle(...args),
  listWatchlistTitlesV2: vi.fn(),
}));
vi.mock("@/playback/watchPlaybackContext", () => ({
  useWatchPlaybackController: () => ({ startPlayback: vi.fn() }),
}));
vi.mock("./SubtitlesPopover", () => ({ default: () => null }));
vi.mock("@/hooks/useAuth", () => ({ useAuth: () => ({ user: { id: 1 } }) }));
vi.mock("sonner", () => ({
  toast: { success: mocks.toastSuccess, error: mocks.toastError },
}));

const heat: RequestMediaDetail = {
  media_type: "movie",
  tmdb_id: 949,
  title: "Heat",
  availability: "missing",
  in_watchlist: false,
  request: { requestable: true },
} as RequestMediaDetail;

function renderBar(item: RequestMediaDetail, queryClient = new QueryClient()) {
  render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <RequestActionBar item={item} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return queryClient;
}

const NOTE = /Adding it to your watchlist also requests it/;

describe("RequestActionBar watchlist toggle", () => {
  beforeEach(() => {
    mocks.createRequest.mockReturnValue({ mutate: vi.fn(), isPending: false });
    mocks.featureStatus.mockReturnValue({
      data: {
        requests_enabled: true,
        allowed: true,
        watchlist_titles_supported: true,
        watchlist_requests: true,
      },
    });
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("offers no watchlist action on a server without watchlist titles", () => {
    mocks.featureStatus.mockReturnValue({ data: { allowed: true } });
    renderBar(heat);
    expect(screen.queryByRole("button", { name: /Watchlist/ })).toBeNull();
    expect(screen.queryByText(NOTE)).toBeNull();
  });

  it("offers no watchlist action while requests are off", () => {
    mocks.featureStatus.mockReturnValue({
      data: { requests_enabled: false, allowed: true, watchlist_titles_supported: true },
    });
    renderBar(heat);
    expect(screen.queryByRole("button", { name: /Watchlist/ })).toBeNull();
  });

  it("adds the title and says it was also requested", async () => {
    const entry: WatchlistTitleEntry = {
      media_type: "movie",
      tmdb_id: 949,
      added_at: "2026-09-30T00:00:00Z",
      request: { requestable: false, status: "pending", requested_by_viewer: true },
    };
    mocks.addWatchlistTitle.mockResolvedValue(entry);
    const queryClient = new QueryClient();
    queryClient.setQueryData(requestKeys.detail("movie", 949), heat);
    renderBar(heat, queryClient);

    const button = screen.getByRole("button", { name: "Add to Watchlist" });
    expect(button).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByText(NOTE)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Settings › Requests" })).toHaveAttribute(
      "href",
      "/settings/requests",
    );

    fireEvent.click(button);

    await waitFor(() =>
      expect(mocks.toastSuccess).toHaveBeenCalledWith("Added to your watchlist and requested", {
        description:
          "We'll let you know when Heat is available. It moves into your watchlist on its own.",
      }),
    );
    expect(mocks.addWatchlistTitle).toHaveBeenCalledWith("movie", 949);
    // The title page reads as on the watchlist straight away.
    expect(
      queryClient.getQueryData<RequestMediaDetail>(requestKeys.detail("movie", 949))?.in_watchlist,
    ).toBe(true);
  });

  it("removes the title and reverts the page when the call fails", async () => {
    let rejectDelete!: (error: Error) => void;
    mocks.deleteWatchlistTitle.mockReturnValue(
      new Promise<void>((_resolve, reject) => {
        rejectDelete = reject;
      }),
    );
    const queryClient = new QueryClient();
    const onList = { ...heat, in_watchlist: true };
    queryClient.setQueryData(requestKeys.detail("movie", 949), onList);
    renderBar(onList, queryClient);

    expect(screen.getByRole("button", { name: "On Watchlist" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.queryByText(NOTE)).toBeNull();

    fireEvent.click(screen.getByRole("button", { name: "On Watchlist" }));
    await waitFor(() =>
      expect(
        queryClient.getQueryData<RequestMediaDetail>(requestKeys.detail("movie", 949))
          ?.in_watchlist,
      ).toBe(false),
    );
    await waitFor(() => expect(mocks.deleteWatchlistTitle).toHaveBeenCalledWith("movie", 949));
    rejectDelete(new Error("Network down"));

    await waitFor(() => expect(mocks.toastError).toHaveBeenCalledWith("Network down"));
    expect(mocks.deleteWatchlistTitle).toHaveBeenCalledWith("movie", 949);
    expect(
      queryClient.getQueryData<RequestMediaDetail>(requestKeys.detail("movie", 949))?.in_watchlist,
    ).toBe(true);
  });
});

describe("RequestActionBar 4K choice", () => {
  const with4K = (overrides: Record<string, unknown> = {}) => {
    mocks.featureStatus.mockReturnValue({
      data: {
        requests_enabled: true,
        allowed: true,
        can_request_4k: true,
        request_4k_available: true,
        ...overrides,
      },
    });
  };

  beforeEach(() => {
    mocks.createRequest.mockReturnValue({ mutate: vi.fn(), isPending: false });
  });
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("requests HD from the main segment and both qualities from the 4K segment", () => {
    const mutate = vi.fn();
    mocks.createRequest.mockReturnValue({ mutate, isPending: false });
    with4K();
    renderBar(heat);

    fireEvent.click(screen.getByRole("button", { name: "Request movie" }));
    expect(mutate.mock.calls.at(0)?.[0]).not.toHaveProperty("qualities");

    fireEvent.click(screen.getByRole("button", { name: "Request 4K" }));
    expect(mutate.mock.calls.at(-1)?.[0]).toMatchObject({ qualities: ["1080p", "2160p"] });
  });

  it("adds only 4K when the title is already requested in HD", () => {
    const mutate = vi.fn();
    mocks.createRequest.mockReturnValue({ mutate, isPending: false });
    with4K();
    renderBar({
      ...heat,
      request: { requestable: false, status: "pending", requested_qualities: ["1080p"] },
    });

    fireEvent.click(screen.getByRole("button", { name: "Request 4K" }));
    expect(mutate.mock.lastCall?.[0]).toMatchObject({ qualities: ["2160p"] });
  });

  it("offers no 4K segment to a non-admin", () => {
    with4K({ can_request_4k: false });
    renderBar(heat);
    expect(screen.getByRole("button", { name: "Request movie" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Request 4K" })).toBeNull();
  });

  it("offers no 4K segment when no connected server takes 4K", () => {
    with4K({ request_4k_available: false });
    renderBar(heat);
    expect(screen.queryByRole("button", { name: "Request 4K" })).toBeNull();
  });

  it("shows the 4K segment as already requested, not clickable, once it is asked for", () => {
    with4K();
    renderBar({
      ...heat,
      request: {
        requestable: false,
        status: "approved",
        state: "approved",
        requested_qualities: ["1080p", "2160p"],
      },
    });

    // The segment stays put as a settled state rather than disappearing, and it
    // wears a tick: there is nothing left to ask for.
    const segment = screen.getByRole("button", { name: "4K requested" });
    expect(segment).toBeDisabled();
    expect(segment.querySelector(".lucide-circle-check")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Request 4K" })).toBeNull();
  });

  it("keeps the 4K segment clickable while the request covers HD alone", () => {
    const mutate = vi.fn();
    mocks.createRequest.mockReturnValue({ mutate, isPending: false });
    with4K();
    renderBar({
      ...heat,
      request: {
        requestable: false,
        status: "pending",
        state: "pending",
        requested_qualities: ["1080p"],
      },
    });

    const segment = screen.getByRole("button", { name: "Request 4K" });
    expect(segment).toBeEnabled();
    expect(segment.querySelector(".lucide-download")).toBeTruthy();
  });
});
