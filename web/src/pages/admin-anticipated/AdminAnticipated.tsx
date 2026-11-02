import { useCallback, useEffect, useMemo, useState } from "react";
import { Search, Trash2 } from "lucide-react";

import {
  fetchAnticipated,
  saveAnticipated,
  searchAnticipated,
  type AnticipatedEntry,
  type AnticipatedList,
  type AnticipatedSearchResult,
} from "@/api/v2/anticipated";
import { V2ProblemError } from "@/api/v2/request";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { tmdbImageURL } from "@/lib/mediaRequests";

/**
 * Admin → Anticipated: the titles the Coming Soon hero shows before the library
 * has them. One ordered list, saved whole, so adding, reordering or removing is
 * a single write guarded by the ETag the page read.
 */
export default function AdminAnticipated() {
  const [list, setList] = useState<AnticipatedList>({ entries: [], max: 0 });
  const [etag, setEtag] = useState<string | null>(null);
  const [draft, setDraft] = useState<AnticipatedEntry[]>([]);
  const [mediaType, setMediaType] = useState<"movie" | "series">("movie");
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<AnticipatedSearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [saving, setSaving] = useState(false);
  const [conflict, setConflict] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const { list: loaded, etag: tag } = await fetchAnticipated();
      setList(loaded);
      setEtag(tag);
      setDraft(loaded.entries.map((e) => ({ media_type: e.media_type, tmdb_id: e.tmdb_id })));
      setConflict(false);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Couldn't load the list.");
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  // A title is on the list once its (media type, TMDB id) pair is.
  const chosen = useMemo(() => new Set(draft.map((e) => `${e.media_type}:${e.tmdb_id}`)), [draft]);

  function add(result: AnticipatedSearchResult) {
    const entry: AnticipatedEntry = { media_type: result.media_type, tmdb_id: result.tmdb_id };
    if (chosen.has(`${entry.media_type}:${entry.tmdb_id}`)) return;
    setDraft((current) => [...current, entry]);
  }

  function remove(index: number) {
    setDraft((current) => current.filter((_, i) => i !== index));
  }

  function move(index: number, by: number) {
    setDraft((current) => {
      const next = [...current];
      const target = index + by;
      if (target < 0 || target >= next.length) return current;
      [next[index], next[target]] = [next[target], next[index]];
      return next;
    });
  }

  async function runSearch() {
    if (!query.trim()) return;
    setSearching(true);
    setError(null);
    try {
      setResults(await searchAnticipated(mediaType, query.trim()));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Search failed.");
    } finally {
      setSearching(false);
    }
  }

  async function save() {
    if (etag === null) return;
    setSaving(true);
    setError(null);
    try {
      const { list: saved, etag: next } = await saveAnticipated(draft, etag);
      setList(saved);
      setEtag(next);
      setDraft(saved.entries.map((e) => ({ media_type: e.media_type, tmdb_id: e.tmdb_id })));
      setConflict(false);
    } catch (err) {
      if (err instanceof V2ProblemError && err.problem.status === 412) {
        setConflict(true);
      } else {
        setError(err instanceof Error ? err.message : "Couldn't save the list.");
      }
    } finally {
      setSaving(false);
    }
  }

  const titles = new Map(list.entries.map((e) => [`${e.media_type}:${e.tmdb_id}`, e] as const));

  return (
    <div className="mx-auto max-w-3xl space-y-6 p-6">
      <header className="space-y-1">
        <h1 className="text-2xl font-semibold">Most Anticipated</h1>
        <p className="text-muted-foreground text-sm">
          Upcoming titles the library does not have yet. Add the ones you want the Coming Soon hero
          to show; each turns into Play the moment its copy lands in the library.
        </p>
      </header>

      {conflict ? (
        <div className="border-border bg-muted/40 rounded-md border p-3 text-sm" role="alert">
          Someone else changed this list.{" "}
          <Button variant="link" onClick={() => void load()}>
            Reload
          </Button>
        </div>
      ) : null}
      {error ? (
        <div
          className="border-destructive/50 text-destructive rounded-md border p-3 text-sm"
          role="alert"
        >
          {error}
        </div>
      ) : null}

      <section className="space-y-3" aria-label="Add a title">
        <div className="flex flex-wrap items-end gap-2">
          <select
            aria-label="Media type"
            className="border-input bg-background h-9 rounded-md border px-2 text-sm"
            value={mediaType}
            onChange={(event) => setMediaType(event.target.value as "movie" | "series")}
          >
            <option value="movie">Movies</option>
            <option value="series">TV shows</option>
          </select>
          <Input
            aria-label="Search TMDB"
            placeholder="Avengers Doomsday"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") void runSearch();
            }}
          />
          <Button onClick={() => void runSearch()} disabled={searching}>
            <Search className="h-4 w-4" />
            Search
          </Button>
        </div>

        {results.length > 0 ? (
          <ul className="divide-border border-border divide-y rounded-md border">
            {results.map((result) => {
              const key = `${result.media_type}:${result.tmdb_id}`;
              const poster = tmdbImageURL(result.poster_path, "w92");
              return (
                <li key={key} className="flex items-center gap-3 p-2">
                  {poster ? (
                    <img src={poster} alt="" className="h-16 w-11 rounded object-cover" />
                  ) : null}
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{result.title}</p>
                    <p className="text-muted-foreground text-xs">
                      {result.year || "—"}
                      {result.release_date ? ` · ${result.release_date}` : ""}
                    </p>
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
                    disabled={chosen.has(key)}
                    onClick={() => add(result)}
                  >
                    {chosen.has(key) ? "Added" : "Add"}
                  </Button>
                </li>
              );
            })}
          </ul>
        ) : null}
      </section>

      <section className="space-y-3" aria-label="The list">
        <h2 className="text-lg font-medium">
          The list <span className="text-muted-foreground text-sm">({draft.length})</span>
        </h2>
        {draft.length === 0 ? (
          <p className="text-muted-foreground text-sm">Nothing yet. Search above to add a title.</p>
        ) : (
          <ol className="divide-border border-border divide-y rounded-md border">
            {draft.map((entry, index) => {
              const known = titles.get(`${entry.media_type}:${entry.tmdb_id}`);
              const key = `${entry.media_type}:${entry.tmdb_id}`;
              return (
                <li key={key} className="flex items-center gap-3 p-2">
                  <span className="text-muted-foreground w-6 text-right text-sm">{index + 1}</span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{known?.title ?? "…"}</p>
                    <p className="text-muted-foreground text-xs">
                      {entry.media_type === "movie" ? "Movie" : "TV"}
                      {known?.release_date ? ` · ${known.release_date}` : ""}
                      {known?.state && known.state !== "active" ? ` · ${known.state}` : ""}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label="Move up"
                    disabled={index === 0}
                    onClick={() => move(index, -1)}
                  >
                    ↑
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label="Move down"
                    disabled={index === draft.length - 1}
                    onClick={() => move(index, 1)}
                  >
                    ↓
                  </Button>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={`Remove ${known?.title ?? entry.tmdb_id}`}
                    onClick={() => remove(index)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </li>
              );
            })}
          </ol>
        )}
        <Button onClick={() => void save()} disabled={saving || etag === null}>
          Save
        </Button>
      </section>
    </div>
  );
}
