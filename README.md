# term-manga

A terminal UI for getting manga onto an e-reader. Search MangaDex and Mangapill together, download chapters as `.cbz`, convert them for a Kindle with KCC, and add them to your Calibre library.

Built in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea), [Bubbles](https://github.com/charmbracelet/bubbles) and [Lip Gloss](https://github.com/charmbracelet/lipgloss).

## Pipeline

```
MangaDex / Mangapill ──► .cbz ──► kcc-c2e ──► .epub ──► calibredb ──► Calibre library
  (search,       (zip of   (KPW5,       (RTL,      (series +
   chapters,      pages)    manga,       cropped)   chapter
   pages)                   crop)                   metadata)
```

Each stage is optional. You can stop after the download and convert later.

## Requirements

- Go 1.27 or newer (to build, per `go.mod`)
- `kcc-c2e`, from Kindle Comic Converter (AUR package `kcc`)
- `7z` (Arch package `7zip`). KCC uses it to extract archives.
- `calibredb`, from Calibre

Download-only use needs none of the external tools. They are checked when you press `c` or `m`.

## Build and run

```bash
# run directly
go run ./cmd/mangapipeline

# or build a binary
go build -o bin/mangapipeline ./cmd/mangapipeline
./bin/mangapipeline
```

## Usage

The app has four screens.

**1. Search.** Type a title and press `enter`.

**2. Results.** Pick a series and press `enter` to load its chapters.

**3. Chapters.** Select what you want, then download.

**4. Download.** Shows progress for each chapter. When the queue finishes, press `enter` or `esc` to go back to the chapter list.

### Keys

| Screen   | Key                | Action                                                        |
|----------|--------------------|---------------------------------------------------------------|
| Search   | `enter`            | Search                                                        |
| Search   | `esc`              | Quit                                                          |
| Results  | `↑`/`↓` or `k`/`j` | Move                                                          |
| Results  | `enter`            | Open chapter list                                             |
| Results  | `esc` or `q`       | Back to search                                                |
| Chapters | `↑`/`↓` or `k`/`j` | Move                                                          |
| Chapters | `pgup`/`pgdn`      | Move 10 chapters                                              |
| Chapters | `space`            | Select or deselect a chapter                                  |
| Chapters | `f`                | Select the chapter under the cursor and every chapter after it |
| Chapters | `a`                | Select all, or clear the selection if all are selected        |
| Chapters | `enter`            | Download only                                                 |
| Chapters | `c`                | Download, convert each chapter to its own EPUB, and import into Calibre |
| Chapters | `m`                | Download, merge into one EPUB per volume, and import into Calibre |
| Chapters | `esc` or `q`       | Back to results                                               |
| Any      | `ctrl+c`           | Quit and cancel the running download                          |

If no chapters are selected, `enter`, `c` and `m` act on the chapter under the cursor.

## Sources

Search queries every source at once and tags each result, for example `[MangaDex] Hunter x Hunter (1998)` or `[Mangapill] Hunter x Hunter (1998)`. If one source is down you still get the other's results. Chapters and pages are fetched from the source the result came from.

| Source    | How                          | Notes                                                                 |
|-----------|------------------------------|-----------------------------------------------------------------------|
| MangaDex  | Public REST API             | Has volume data, so `m` can group by volume.                          |
| Mangapill | HTML scraping (no API)      | No volume data. May break if the site changes its markup. Images need a `Referer` header, which the app sends. |

Sources implement the `source.Source` interface in `internal/source/source.go`. Both sources host unlicensed scanlations. Comick was tried and dropped: its API lists chapters but returns no page images.

## Merging into volumes (`m`)

`m` downloads the selected chapters, then fuses them into EPUBs with `kcc-c2e --filefusion` and imports each into Calibre as part of one series, indexed by volume:

- One EPUB per MangaDex volume (`<Series> - Vol. 3.epub`), written to `~/Manga/<series>/epub/`.
- Chapters with no volume number are chunked 12 at a time (`<Series> - Ch. 41-50.epub`). Volumes of more than 12 chapters are split into parts (`Vol. 3 pt. 1`).
- A file over 200 MB is reported as an error and not imported. Very large omnibuses make Kindles lag.
- An existing merged EPUB is reused. If a volume gains chapters later, delete its EPUB to rebuild it.

In the chapter list, a green `✓` after a chapter means its `.cbz` is already in `~/Manga`. The marks refresh when you return from a download.

## Where files go

Downloads are kept in `~/Manga`, in one folder per series:

```
~/Manga/
└── Yotsuba to!/
    ├── Yotsuba to! - Ch. 1.cbz
    ├── Yotsuba to! - Ch. 2.cbz
    └── epub/
        ├── Yotsuba to! - Ch. 1.epub
        └── Yotsuba to! - Ch. 2.epub
```

Characters that are illegal in file names (`/ \ : * ? " < > |`) are replaced with `_`.

Merged volumes (`m`) are written to the same `epub/` folder.

Re-running is safe. An existing `.cbz` or `.epub` is reused rather than rebuilt, and `calibredb` skips books it already has.

## Conversion settings

KCC runs with:

```
kcc-c2e -p KPW5 -m -c 2 -f EPUB --nokepub -t "<Series> - Ch. <N>" -o <out.epub> <in.cbz>
```

| Flag            | Meaning                                                                  |
|-----------------|--------------------------------------------------------------------------|
| `-p KPW5`       | Kindle Paperwhite 5 / Signature Edition profile (1236x1648)              |
| `-m`            | Manga mode: right-to-left reading and page splitting                     |
| `-c 2`          | Auto-crop margins and page numbers                                       |
| `-f EPUB`       | EPUB output                                                              |
| `--nokepub`     | Write a plain `.epub` instead of `.kepub.epub`                           |

The profile is the `Profile` constant in `internal/pipeline/pipeline.go`. Other KCC profiles (`KPW6`, `KoLC`, ...) are listed by `kcc-c2e --help`.

## Calibre import

Chapters are added to your default Calibre library (the one Calibre last used, normally `~/Calibre Library`) with:

| Field        | Value                   |
|--------------|-------------------------|
| Title        | `<Series> - Ch. <N>`, or `<Series> - Vol. <N>` with `m` |
| Author       | `MangaDex` (set for every source) |
| Series       | The manga title         |
| Series index | The chapter number, or the volume number with `m` |
| Tags         | `manga`                 |

If the Calibre desktop app is open, `calibredb` may refuse to write to the library. Close Calibre and retry.

## What gets listed

The rules below are for MangaDex. Mangapill lists every chapter titled "Chapter N", with duplicate numbers collapsed.

- **Language:** English only.
- **Content rating:** safe and suggestive only.
- **Duplicate chapters:** if several scanlation groups uploaded the same chapter number, only the first is kept.
- **Oneshots:** chapters with no chapter number are skipped.
- **External chapters:** chapters that only link to an outside reader are skipped, because there are no pages to download.

### Official series are often not downloadable

Many official releases on MangaDex are links to the publisher's reader, such as MANGA Plus, and have no hosted pages. For example, the main "Death Note" entry has 5 chapters, all external. When every chapter of a series is external, the app shows an error saying so. Try a different entry for the same series (such as a fan-translated or "Official Colored" one).

Series with hosted chapters that worked in testing: **Yotsuba&!** (search "Yotsuba") and **Hunter x Hunter**.

## Development

```bash
go test -v ./...          # unit tests
go test -race ./...       # with the race detector
gofmt -s -w .             # format
golangci-lint run         # lint
```

### Layout

```
cmd/mangapipeline/     entry point
internal/source/       Source interface shared by all sites
internal/mangadex/     MangaDex API client: search, chapter feed, page URLs
internal/mangapill/    Mangapill scraper: search, chapter list, page URLs
internal/download/     parallel page downloads into .cbz, path and file-name helpers
internal/pipeline/     kcc-c2e and calibredb wrappers, volume grouping
internal/tui/          Bubble Tea model and views
```

## Notes

Only download manga you have the right to read in this way. Respect each site's terms of service and rate limits. Pages are downloaded 6 at a time per chapter (`pageWorkers` in `internal/download/cbz.go`); lower it if a site starts rejecting requests.
