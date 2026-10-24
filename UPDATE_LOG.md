# Update Log

## 2026-10-23

### Faster downloads
- Pages in a chapter are now fetched by 6 concurrent workers instead of one at a time, and written to the `.cbz` in page order (`internal/download/cbz.go`).
- The HTTP client keeps up to 12 idle connections per host (Go's default is 2) and uses HTTP/2, so parallel fetches reuse connections.
- Each page still retries 3 times with backoff; one failed page cancels the rest.
- Fixed a deadlock in the first version of this change: cancelling a download (for example Ctrl+C) could hang forever. The writer now also watches for cancellation.

### Merge chapters into volumes (`m` key)
- On the chapter screen, `m` downloads the selection, merges it into volume EPUBs with `kcc-c2e --filefusion`, and imports each into Calibre.
- One EPUB per MangaDex volume. Chapters with no volume are chunked 12 at a time (`Ch. 41-50`). Volumes over 12 chapters are split into parts (`Vol. 3 pt. 1`).
- Files over 200 MB are reported as an error and are not imported, because large omnibuses lag Kindles.
- Volumes are added to Calibre as one series, indexed by volume number.
- Existing volume EPUBs are reused, so a volume that gained chapters needs its EPUB deleted to rebuild.
- `c` still converts and imports one chapter at a time.

### Multiple sources (Mangapill added)
- New `source.Source` interface (`internal/source/source.go`). MangaDex now implements it, and the TUI no longer depends on MangaDex types.
- New Mangapill source (`internal/mangapill`). It scrapes HTML because the site has no API, so it can break if the markup changes. It has no volume data, so `m` chunks its chapters 12 at a time.
- Search queries every source in parallel. Results are tagged `[MangaDex]` or `[Mangapill]`. A source that fails is skipped unless all of them fail.
- Chapters and page downloads go through the source the chosen result came from.
- `download.CBZ` takes a `referer` argument, since Mangapill's image CDN returns 403 without one. Page URLs with a query string (`?t=...`) now keep the right file extension.
- Comick was checked and rejected: its API lists chapters but returns no page images.
- Tested the Mangapill client against the live site (search, 420 chapters, 38 pages for chapter 1) and added a unit test with a fake server.

### Chapter list
- `f` selects the chapter under the cursor and every chapter after it.
- Chapters already downloaded show a green `✓`. The marks refresh when you come back from a download.

### Fixes
- An oversized merged EPUB (over 200 MB) was reused and imported on the next run, because the size check only ran right after a build. The check now also runs on reused files, and an oversized EPUB is deleted.
- Pages fetched ahead of the writer are capped at 12 per chapter, so a slow page can't make a long chapter pile up in memory.
- Part indexes for split volumes use steps of 0.01, so they can't spill into the next volume's number.
- MangaDex 429 (rate limit) responses are retried after the reset time. Chapters already on disk no longer ask MangaDex for page URLs.
- The download screen crashed when the merge started (it indexed past the end of the queue), which killed the merge.
- The `m` key did nothing at first because the key handler only matched `enter` and `c`. It now matches `m` too.

### Verification
- `go vet` clean; `go test -race ./...` passes.
- `kcc-c2e --filefusion` was run by hand on two small test CBZs and produced one merged EPUB.
- Run by hand in the TUI: parallel downloads and the `m` merge both work.

## 2026-10-20 and earlier

Initial commit, plus uncommitted work present at the start of this session:
- Bubble Tea TUI with four screens: search, results, chapters, download.
- MangaDex client: title search (English; safe and suggestive ratings), chapter feed with duplicate numbers collapsed, and page URLs from the at-home server. External-only chapters are counted and skipped.
- `.cbz` downloader that writes to a `.part` file and renames on success, so failures leave no partial archive. Output goes to `~/Manga/<series>/`.
- KCC conversion (Kindle Paperwhite 5 profile, manga mode, auto-crop, EPUB) and Calibre import via `calibredb`.
- Unit tests for the MangaDex client and the downloader; README.

## Ideas not done yet
- Download chapter N+1 while KCC converts chapter N.
- Optional MangaDex `data-saver` image quality.
- Optional omnibus mode (3 to 5 volumes per file, under about 200 MB).
