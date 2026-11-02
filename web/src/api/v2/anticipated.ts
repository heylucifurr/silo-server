/**
 * The curated "Most Anticipated" list: the titles an admin hand-picks for the
 * Coming Soon hero, and the TMDB search that fills it.
 */
import { requiredETag, withETag } from "./etag";
import { v2 } from "./request";
import type { components } from "./schema";

export type AnticipatedTitle = components["schemas"]["AdminAnticipatedTitle"];
export type AnticipatedEntry = components["schemas"]["AdminAnticipatedEntryBody"];
export type AnticipatedSearchResult = components["schemas"]["AdminAnticipatedSearchResult"];

export interface AnticipatedList {
  entries: AnticipatedTitle[];
  /** The most entries the list holds. */
  max: number;
}

function toList(body: components["schemas"]["AdminAnticipatedList"]): AnticipatedList {
  return { entries: body.entries ?? [], max: body.max ?? 0 };
}

/** The curated list, with the validator its next save must send as If-Match. */
export async function fetchAnticipated(): Promise<{ list: AnticipatedList; etag: string }> {
  const { body, etag } = await withETag("GET /api/v2/admin/anticipated");
  return { list: toList(body), etag };
}

/**
 * Replaces the whole list in the given order. A 412 means another admin saved
 * first; the caller reloads rather than clobbering.
 */
export async function saveAnticipated(
  entries: AnticipatedEntry[],
  etag: string,
): Promise<{ list: AnticipatedList; etag: string }> {
  const { body, etag: next } = await withETag("PUT /api/v2/admin/anticipated", {
    headers: { "If-Match": requiredETag(etag) },
    body: { entries },
  });
  return { list: toList(body), etag: next };
}

/** TMDB search for titles to add to the list. */
export async function searchAnticipated(
  mediaType: "movie" | "series",
  query: string,
): Promise<AnticipatedSearchResult[]> {
  const body = await v2("POST /api/v2/admin/anticipated/search", {
    query: { media_type: mediaType, q: query },
  });
  return body.items ?? [];
}
