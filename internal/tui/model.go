// Package tui is the Bubble Tea front end.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ashwinnair/term-manga/internal/download"
	"github.com/ashwinnair/term-manga/internal/mangadex"
	"github.com/ashwinnair/term-manga/internal/pipeline"
)

type screen int

const (
	screenSearch screen = iota
	screenResults
	screenChapters
	screenDownload
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	okStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
)

// Messages.
type (
	searchDoneMsg   struct{ manga []mangadex.Manga }
	chaptersDoneMsg struct {
		chapters []mangadex.Chapter
		external int
	}
	errMsg          struct{ err error }
	pageProgressMsg struct{ done, total int }
	stageMsg        struct{ name string }
	chapterDoneMsg  struct {
		number string
		err    error
	}
	mergeDoneMsg struct{ errs []error }
)

type Model struct {
	client *mangadex.Client
	root   string

	screen  screen
	input   textinput.Model
	spin    spinner.Model
	bar     progress.Model
	loading bool
	err     error
	width   int
	height  int

	results []mangadex.Manga
	cursor  int
	manga   mangadex.Manga

	chapters []mangadex.Chapter
	selected map[int]bool

	// download state
	queue    []mangadex.Chapter
	qIdx     int
	pageDone int
	pageTot  int
	stage    string
	convert  bool
	merge    bool // convert per-volume merged EPUBs instead of one per chapter
	okChs    []mangadex.Chapter
	log      []string
	progCh   chan tea.Msg
	cancel   context.CancelFunc
	finished bool
}

func New() (Model, error) {
	root, err := download.Root()
	if err != nil {
		return Model{}, err
	}
	ti := textinput.New()
	ti.Placeholder = "search MangaDex..."
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 40

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return Model{
		client: mangadex.New(),
		root:   root,
		input:  ti,
		spin:   sp,
		bar:    progress.New(progress.WithDefaultGradient(), progress.WithWidth(40)),
	}, nil
}

func (m Model) Init() tea.Cmd { return tea.Batch(textinput.Blink, m.spin.Tick) }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		model, cmd := m.bar.Update(msg)
		m.bar = model.(progress.Model)
		return m, cmd

	case errMsg:
		m.loading, m.err = false, msg.err
		return m, nil

	case searchDoneMsg:
		m.loading = false
		m.results, m.cursor = msg.manga, 0
		if len(m.results) == 0 {
			m.err = fmt.Errorf("no results")
			return m, nil
		}
		m.screen = screenResults
		return m, nil

	case chaptersDoneMsg:
		m.loading = false
		m.chapters, m.cursor = msg.chapters, 0
		m.selected = map[int]bool{}
		if len(m.chapters) == 0 {
			if msg.external > 0 {
				m.err = fmt.Errorf("all %d English chapters are external links (e.g. MANGA Plus) and can't be downloaded; try another entry", msg.external)
			} else {
				m.err = fmt.Errorf("no English chapters found")
			}
			return m, nil
		}
		m.screen = screenChapters
		return m, nil

	case stageMsg:
		m.stage = msg.name
		return m, waitProgress(m.progCh)

	case pageProgressMsg:
		m.stage = "downloading"
		m.pageDone, m.pageTot = msg.done, msg.total
		cmd := m.bar.SetPercent(float64(msg.done) / float64(msg.total))
		return m, tea.Batch(cmd, waitProgress(m.progCh))

	case chapterDoneMsg:
		if msg.err != nil {
			m.log = append(m.log, errStyle.Render("✗ Ch. "+msg.number+": "+msg.err.Error()))
		} else {
			m.log = append(m.log, okStyle.Render("✓ Ch. "+msg.number))
			m.okChs = append(m.okChs, m.queue[m.qIdx])
		}
		m.qIdx++
		if m.qIdx >= len(m.queue) {
			if m.merge && len(m.okChs) > 0 {
				m.pageDone, m.pageTot, m.stage = 0, 0, "merging"
				return m, m.mergeCmd()
			}
			m.finished = true
			return m, nil
		}
		m.pageDone, m.pageTot, m.stage = 0, 0, ""
		dl := m.downloadCmd()
		return m, tea.Batch(m.bar.SetPercent(0), dl)

	case mergeDoneMsg:
		for _, err := range msg.errs {
			m.log = append(m.log, errStyle.Render("✗ "+err.Error()))
		}
		if len(msg.errs) == 0 {
			m.log = append(m.log, okStyle.Render("✓ merged volumes imported to Calibre"))
		}
		m.finished = true
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.screen == screenSearch {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	if m.loading {
		return m, nil
	}

	switch m.screen {
	case screenSearch:
		switch key {
		case "enter":
			q := strings.TrimSpace(m.input.Value())
			if q == "" {
				return m, nil
			}
			m.loading, m.err = true, nil
			return m, m.searchCmd(q)
		case "esc":
			return m, tea.Quit
		}
		m.err = nil
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd

	case screenResults:
		switch key {
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(len(m.results)-1, m.cursor+1)
		case "enter":
			m.manga = m.results[m.cursor]
			m.loading, m.err = true, nil
			return m, m.chaptersCmd(m.manga.ID)
		case "esc", "q":
			m.screen, m.err = screenSearch, nil
		}

	case screenChapters:
		switch key {
		case "up", "k":
			m.cursor = max(0, m.cursor-1)
		case "down", "j":
			m.cursor = min(len(m.chapters)-1, m.cursor+1)
		case "pgup":
			m.cursor = max(0, m.cursor-10)
		case "pgdown":
			m.cursor = min(len(m.chapters)-1, m.cursor+10)
		case " ":
			m.selected[m.cursor] = !m.selected[m.cursor]
		case "a":
			all := len(m.selected) == len(m.chapters)
			m.selected = map[int]bool{}
			if !all {
				for i := range m.chapters {
					m.selected[i] = true
				}
			}
		case "enter", "c":
			m.convert = key == "c"
			for i, c := range m.chapters {
				if m.selected[i] {
					m.queue = append(m.queue, c)
				}
			}
			if len(m.queue) == 0 {
				m.queue = []mangadex.Chapter{m.chapters[m.cursor]}
			}
			if m.convert {
				if err := pipeline.CheckTools(); err != nil {
					m.queue, m.err = nil, err
					return m, nil
				}
			}
			m.err = nil
			m.screen, m.qIdx, m.log, m.finished = screenDownload, 0, nil, false
			dl := m.downloadCmd()
			return m, dl
		case "esc", "q":
			m.screen, m.cursor, m.err = screenResults, 0, nil
		}

	case screenDownload:
		if m.finished && (key == "esc" || key == "q" || key == "enter") {
			m.queue, m.screen, m.cursor = nil, screenChapters, 0
		}
	}
	return m, nil
}

func (m Model) searchCmd(q string) tea.Cmd {
	return func() tea.Msg {
		r, err := m.client.Search(context.Background(), q)
		if err != nil {
			return errMsg{err}
		}
		return searchDoneMsg{r}
	}
}

func (m Model) chaptersCmd(id string) tea.Cmd {
	return func() tea.Msg {
		c, ext, err := m.client.Chapters(context.Background(), id)
		if err != nil {
			return errMsg{err}
		}
		return chaptersDoneMsg{c, ext}
	}
}

// mergeCmd fuses the downloaded chapters into volumes with KCC and imports each
// into Calibre. Progress is reported through m.progCh, whose waiter is still armed.
func (m *Model) mergeCmd() tea.Cmd {
	series, root, progCh := m.manga.Title, m.root, m.progCh
	vols := pipeline.GroupVolumes(m.okChs, pipeline.MaxVolumeChapters)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	return func() tea.Msg {
		defer cancel()
		var errs []error
		for _, v := range vols {
			progCh <- stageMsg{"merging " + v.Label}
			cbzs := make([]string, len(v.Chapters))
			for i, c := range v.Chapters {
				cbzs[i] = download.ChapterPath(root, series, c.Number)
			}
			title := series + " - " + v.Label
			out := pipeline.MergedPath(filepath.Dir(cbzs[0]), download.SafeName(series), download.SafeName(v.Label))
			if err := pipeline.ConvertMerged(ctx, cbzs, title, out); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", v.Label, err))
				continue
			}
			progCh <- stageMsg{"importing " + v.Label}
			if err := pipeline.Ingest(ctx, out, title, series, strconv.FormatFloat(v.Index, 'f', -1, 64)); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", v.Label, err))
			}
		}
		return mergeDoneMsg{errs}
	}
}

// downloadCmd downloads the current queue entry. Page progress flows through
// m.progCh, which is created once per queue run.
func (m *Model) downloadCmd() tea.Cmd {
	first := m.qIdx == 0
	if first {
		// One fresh channel and one waiter per run; the waiter re-arms itself on
		// every progress message, so later chapters must not start another.
		m.progCh = make(chan tea.Msg, 16)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	ch := m.queue[m.qIdx]
	series, client, root, progCh := m.manga.Title, m.client, m.root, m.progCh
	convert := m.convert && !m.merge // merge mode converts once, after all downloads

	run := func() tea.Msg {
		defer cancel()
		urls, err := client.PageURLs(ctx, ch.ID)
		if err != nil {
			return chapterDoneMsg{ch.Number, err}
		}
		dest := download.ChapterPath(root, series, ch.Number)
		if _, statErr := os.Stat(dest); statErr != nil {
			err = download.CBZ(ctx, dest, urls, func(done, total int) {
				select {
				case progCh <- pageProgressMsg{done, total}:
				default: // UI is behind; drop the update
				}
			})
			if err != nil {
				return chapterDoneMsg{ch.Number, err}
			}
		}
		if !convert {
			return chapterDoneMsg{ch.Number, nil}
		}
		title := fmt.Sprintf("%s - Ch. %s", series, ch.Number)
		progCh <- stageMsg{"converting"}
		epub, err := pipeline.Convert(ctx, dest, title)
		if err != nil {
			return chapterDoneMsg{ch.Number, err}
		}
		progCh <- stageMsg{"importing to Calibre"}
		return chapterDoneMsg{ch.Number, pipeline.Ingest(ctx, epub, title, series, ch.Number)}
	}
	if first {
		return tea.Batch(run, waitProgress(progCh))
	}
	return run
}

func waitProgress(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Manga Pipeline") + "\n\n")

	switch m.screen {
	case screenSearch:
		b.WriteString(m.input.View() + "\n\n")
		if m.loading {
			b.WriteString(m.spin.View() + " searching...\n")
		}
		b.WriteString(dimStyle.Render("enter search • esc quit"))

	case screenResults:
		b.WriteString(fmt.Sprintf("Results (%d)\n\n", len(m.results)))
		start, end := window(m.cursor, len(m.results), m.listHeight())
		for i := start; i < end; i++ {
			r := m.results[i]
			line := r.Title
			if r.Year > 0 {
				line += dimStyle.Render(" (" + strconv.Itoa(r.Year) + ")")
			}
			b.WriteString(row(i == m.cursor, "", line))
		}
		if m.loading {
			b.WriteString("\n" + m.spin.View() + " loading chapters...\n")
		}
		b.WriteString("\n" + dimStyle.Render("↑/↓ move • enter chapters • esc back"))

	case screenChapters:
		b.WriteString(fmt.Sprintf("%s — %d chapters, %d selected\n\n", m.manga.Title, len(m.chapters), len(m.selected)))
		start, end := window(m.cursor, len(m.chapters), m.listHeight())
		for i := start; i < end; i++ {
			c := m.chapters[i]
			mark := "[ ] "
			if m.selected[i] {
				mark = selectedStyle.Render("[x] ")
			}
			line := "Ch. " + c.Number
			if c.Title != "" {
				line += dimStyle.Render(" " + c.Title)
			}
			b.WriteString(row(i == m.cursor, mark, line))
		}
		b.WriteString("\n" + dimStyle.Render("space toggle • a all • enter download • c per-chapter convert+Calibre • m merge into volumes+Calibre (cursor chapter if none selected) • esc back"))

	case screenDownload:
		fmt.Fprintf(&b, "Downloading to %s\n\n", m.root)
		for _, l := range m.log {
			b.WriteString(l + "\n")
		}
		if m.finished {
			b.WriteString("\n" + okStyle.Render("Done.") + " " + dimStyle.Render("enter/esc back"))
		} else {
			cur := m.queue[m.qIdx]
			fmt.Fprintf(&b, "\n%s Ch. %s (%d/%d)  %s", m.spin.View(),
				cur.Number, m.qIdx+1, len(m.queue), m.stage)
			if m.stage == "downloading" || m.stage == "" {
				fmt.Fprintf(&b, "  page %d/%d\n%s", m.pageDone, m.pageTot, m.bar.View())
			}
		}
	}

	if m.err != nil {
		b.WriteString("\n\n" + errStyle.Render("error: "+m.err.Error()))
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String()) + "\n"
}

func row(active bool, prefix, text string) string {
	if active {
		return cursorStyle.Render("> ") + prefix + text + "\n"
	}
	return "  " + prefix + text + "\n"
}

func (m Model) listHeight() int {
	if m.height == 0 {
		return 15
	}
	return max(5, m.height-10)
}

// window returns the [start,end) slice of a list of n items that keeps cursor visible.
func window(cursor, n, h int) (int, int) {
	if n <= h {
		return 0, n
	}
	start := min(max(0, cursor-h/2), n-h)
	return start, start + h
}
