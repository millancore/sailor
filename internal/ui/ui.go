package ui

import (
	"fmt"
	"os"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
	"charm.land/lipgloss/v2/table"
)

// Color palette
var (
	primary  = compat.AdaptiveColor{Light: lipgloss.Color("#5A56E0"), Dark: lipgloss.Color("#7571F9")}
	success  = compat.AdaptiveColor{Light: lipgloss.Color("#02BA84"), Dark: lipgloss.Color("#02BF87")}
	errColor = compat.AdaptiveColor{Light: lipgloss.Color("#FF4672"), Dark: lipgloss.Color("#ED567A")}
	warn     = compat.AdaptiveColor{Light: lipgloss.Color("#F59E0B"), Dark: lipgloss.Color("#FBBF24")}
	muted    = lipgloss.Color("243")
	border   = lipgloss.Color("238")
)

var (
	successStyle = lipgloss.NewStyle().Foreground(success)
	errStyle     = lipgloss.NewStyle().Foreground(errColor)
	warnStyle    = lipgloss.NewStyle().Foreground(warn)
	primaryStyle = lipgloss.NewStyle().Foreground(primary)
	mutedStyle   = lipgloss.NewStyle().Foreground(muted)
	boldStyle    = lipgloss.NewStyle().Bold(true)
)

// Status printers

func Info(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("  %s  %s\n", primaryStyle.Render("ℹ"), msg)
}

func Success(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("  %s  %s\n", successStyle.Render("✔"), msg)
}

func Warn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("  %s  %s\n", warnStyle.Render("⚠"), msg)
}

func Error(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "  %s  %s\n", errStyle.Render("✖"), msg)
}

func Step(n, total int, label string) {
	fmt.Println()
	step := primaryStyle.Render(fmt.Sprintf("%d/%d", n, total))
	fmt.Printf("  %s  %s\n", step, boldStyle.Render(label))
}

// Inline formatters

func Bold(s string) string  { return boldStyle.Render(s) }
func Dim(s string) string   { return mutedStyle.Render(s) }
func Green(s string) string { return successStyle.Render(s) }
func Red(s string) string   { return errStyle.Render(s) }
func Yellow(s string) string { return warnStyle.Render(s) }
func Cyan(s string) string  { return primaryStyle.Render(s) }

func Link(url, text string) string {
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, text)
}

// Table renders a styled lipgloss table with rounded borders and violet header.
func Table(headers []string, rows [][]string) string {
	headerStyle := lipgloss.NewStyle().
		Foreground(primary).
		Bold(true).
		Padding(0, 1)

	cellStyle := lipgloss.NewStyle().
		Padding(0, 1)

	borderStyle := lipgloss.NewStyle().
		Foreground(border)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderRow(true).
		BorderStyle(borderStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		}).
		Headers(headers...).
		Rows(rows...)

	return t.Render()
}

// Section prints a bold section header with surrounding whitespace.
func Section(label string) {
	fmt.Println()
	fmt.Printf("  %s\n", boldStyle.Render(label))
	fmt.Println()
}

// Banner returns the ASCII art banner.
func Banner() string {
	art := ` ____    _    ___ _     ___  ____
/ ___|  / \  |_ _| |   / _ \|  _ \
\___ \ / _ \  | || |  | | | | |_) |
 ___) / ___ \ | || |__| |_| |  _ <
|____/_/   \_\___|_____\___/|_| \_\`

	tagline := "  Laravel Sail worktree manager"

	return lipgloss.NewStyle().Foreground(primary).Render(art) + "\n" +
		mutedStyle.Render(tagline) + "\n"
}

// SummaryBox renders a rounded bordered box with key/value pairs.
func SummaryBox(title string, lines []string) string {
	content := boldStyle.Render(title) + "\n\n"
	for _, l := range lines {
		content += "  " + l + "\n"
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primary).
		Padding(1, 2).
		Render(content)
}

// ── Spinner ────────────────────────────────────────────────────────────────

type doneMsg struct{ err error }

type spinnerModel struct {
	spinner spinner.Model
	label   string
	fn      func() error
	done    bool
	err     error
}

func (m spinnerModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { return doneMsg{m.fn()} },
	)
}

func (m spinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case doneMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m spinnerModel) View() tea.View {
	if m.done {
		if m.err != nil {
			return tea.NewView(fmt.Sprintf("  %s  %s\n", errStyle.Render("✖"), m.label))
		}
		return tea.NewView(fmt.Sprintf("  %s  %s\n", successStyle.Render("✔"), m.label))
	}
	return tea.NewView(fmt.Sprintf("  %s  %s\n", primaryStyle.Render(m.spinner.View()), m.label))
}

// Spin runs fn() while showing an animated spinner. Returns fn's error.
func Spin(label string, fn func() error) error {
	var (
		once    sync.Once
		fnErr   error
	)
	wrapped := func() error {
		once.Do(func() { fnErr = fn() })
		return fnErr
	}

	s := spinner.New(spinner.WithStyle(primaryStyle))
	s.Spinner = spinner.Dot

	m := spinnerModel{spinner: s, label: label, fn: wrapped}

	p := tea.NewProgram(m, tea.WithoutSignalHandler())
	finalModel, err := p.Run()
	if err != nil {
		// BubbleTea failed to start; run fn if it hasn't run yet.
		once.Do(func() { fnErr = fn() })
		return fnErr
	}

	time.Sleep(10 * time.Millisecond) // allow terminal to flush final frame

	if fm, ok := finalModel.(spinnerModel); ok {
		return fm.err
	}
	return fnErr
}

// ── Confirm ────────────────────────────────────────────────────────────────

type confirmModel struct {
	title       string
	description string
	confirmed   bool
	answered    bool
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "y", "Y":
			m.confirmed = true
			m.answered = true
			return m, tea.Quit
		case "n", "N", "q", "ctrl+c", "esc":
			m.confirmed = false
			m.answered = true
			return m, tea.Quit
		case "enter":
			m.answered = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m confirmModel) View() tea.View {
	if m.answered {
		answer := Red("No")
		if m.confirmed {
			answer = Green("Yes")
		}
		return tea.NewView(fmt.Sprintf("  %s %s %s\n", boldStyle.Render(m.title), Dim("→"), answer))
	}

	s := fmt.Sprintf("\n  %s\n", boldStyle.Render(m.title))
	if m.description != "" {
		s += fmt.Sprintf("  %s\n", mutedStyle.Render(m.description))
	}
	yes := successStyle.Render("y")
	no := errStyle.Render("n")
	s += fmt.Sprintf("\n  %s/%s  ", yes, no)
	return tea.NewView(s)
}

// Confirm shows a yes/no prompt and returns the user's answer.
func Confirm(title, description string) (bool, error) {
	m := confirmModel{title: title, description: description}
	p := tea.NewProgram(m, tea.WithoutSignalHandler())
	final, err := p.Run()
	if err != nil {
		return false, err
	}
	if fm, ok := final.(confirmModel); ok {
		return fm.confirmed, nil
	}
	return false, nil
}

// ── Select ─────────────────────────────────────────────────────────────────

// SelectOption is a single option in a Select prompt.
type SelectOption struct {
	Label       string
	Description string
	Value       string
}

type selectModel struct {
	title   string
	options []SelectOption
	cursor  int
	chosen  string
	done    bool
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "enter", " ":
			m.chosen = m.options[m.cursor].Value
			m.done = true
			return m, tea.Quit
		case "q", "ctrl+c", "esc":
			m.done = true
			return m, tea.Quit
		default:
			if len(msg.String()) == 1 {
				ch := msg.String()[0]
				if ch >= '1' && ch <= '9' {
					idx := int(ch-'1')
					if idx < len(m.options) {
						m.chosen = m.options[idx].Value
						m.done = true
						return m, tea.Quit
					}
				}
			}
		}
	}
	return m, nil
}

func (m selectModel) View() tea.View {
	if m.done {
		label := ""
		for _, o := range m.options {
			if o.Value == m.chosen {
				label = o.Label
				break
			}
		}
		return tea.NewView(fmt.Sprintf("  %s %s %s\n",
			boldStyle.Render(m.title), Dim("→"), Green(label)))
	}

	s := fmt.Sprintf("\n  %s\n\n", boldStyle.Render(m.title))
	for i, opt := range m.options {
		cursor := "  "
		labelStyle := lipgloss.NewStyle()
		descStyle := mutedStyle
		if i == m.cursor {
			cursor = primaryStyle.Render("❯ ")
			labelStyle = primaryStyle
			descStyle = lipgloss.NewStyle().Foreground(primary).Faint(true)
		}
		num := mutedStyle.Render(fmt.Sprintf("%d. ", i+1))
		s += fmt.Sprintf("  %s%s%s\n", cursor, num, labelStyle.Render(opt.Label))
		if opt.Description != "" {
			s += fmt.Sprintf("      %s\n", descStyle.Render(opt.Description))
		}
	}
	s += fmt.Sprintf("\n  %s\n", mutedStyle.Render("↑/↓ navigate • enter select • 1-9 quick pick"))
	return tea.NewView(s)
}

// Select shows a navigable list of options and returns the chosen value.
func Select(title string, options []SelectOption, defaultValue string) (string, error) {
	cursor := 0
	for i, o := range options {
		if o.Value == defaultValue {
			cursor = i
			break
		}
	}

	m := selectModel{title: title, options: options, cursor: cursor}
	p := tea.NewProgram(m, tea.WithoutSignalHandler())
	final, err := p.Run()
	if err != nil {
		return defaultValue, err
	}
	if fm, ok := final.(selectModel); ok && fm.chosen != "" {
		return fm.chosen, nil
	}
	return defaultValue, nil
}
