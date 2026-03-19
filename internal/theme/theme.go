package theme

import "charm.land/lipgloss/v2"

// ── Core colors ──────────────────────────────────────────

var (
	Title = lipgloss.NewStyle().Bold(true).
		Foreground(lipgloss.Color("0")).
		Background(lipgloss.Color("15")).Padding(0, 2)

	Header  = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	Label   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	Value   = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	Mono    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	Dim     = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	Grayed  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	Good    = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	Success = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	Warn    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	Warning = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	Action  = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	Footer  = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
)

// ── Branded ──────────────────────────────────────────────

var (
	Bitcoin   = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	Lightning = lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Bold(true)
)

// ── Status indicators ────────────────────────────────────

var (
	GreenDot = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	RedDot   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// ── Tabs ─────────────────────────────────────────────────

var (
	ActiveTab = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("15")).Padding(0, 2)
	InactiveTab = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("236")).Padding(0, 2)
)

// ── Borders ──────────────────────────────────────────────

var (
	Box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("245"))
	SelectedBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("220"))
	NormalBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("245"))
	GrayedBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))
)

// ── Install progress ─────────────────────────────────────

var (
	ProgTitle = lipgloss.NewStyle().Bold(true).
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("15")).Padding(0, 2)
	ProgBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("245")).Padding(1, 2)
	ProgDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	ProgRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	ProgPending = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	ProgFail    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

// ── Setup wizard ─────────────────────────────────────────

var (
	SetupSection    = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	SetupSelected   = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	SetupUnselected = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	SetupValue      = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	SummaryKey      = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).
			Width(16).Align(lipgloss.Right)
	SummaryVal = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
)

// ── Layout constants ─────────────────────────────────────

const (
	ContentWidth = 76
	BoxHeight    = 24
)
