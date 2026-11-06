# Rating source marks

Artwork for the rating sources whose marks the detail hero draws as a logo:

| File | Source | Rating source name |
|---|---|---|
| `imdb.svg` | IMDb | `imdb` |
| `rt-critic.svg` | Rotten Tomatoes (tomato) | `rt_critic` |
| `rt-audience.svg` | Rotten Tomatoes (popcorn) | `rt_audience` |

These are the score owners' trademarks, not Silo's, and are not covered by this
repository's license. Supplied by the fork owner and used here at their
direction.

Upstream Silo draws every rating mark as plain text and uses only TMDB's
approved logo (`../tmdb/`), because IMDb and Rotten Tomatoes restrict how others
display their logos; see `docs/catalog-api.md`. This fork's web client draws
these three instead, so that sentence no longer describes it. Keep the files
unmodified (colours, proportions) and less prominent than the app's own
branding.
