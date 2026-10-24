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

### Verification
- `go vet` clean; `go test -race ./...` passes.
- `kcc-c2e --filefusion` was run by hand on two small test CBZs and produced one merged EPUB.
- Not yet run end to end against MangaDex and Calibre.

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
