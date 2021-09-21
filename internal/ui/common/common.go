package common

import (
	"github.com/charmbracelet/lipgloss"
)

const ErrorEmoji = "❗️"
const WarningEmoji = "❓"

// Colors for dark and light backgrounds.
var (
	Indigo       = lipgloss.AdaptiveColor{Dark: "#7571F9", Light: "#5A56E0"}
	SubtleIndigo = lipgloss.AdaptiveColor{Dark: "#514DC1", Light: "#7D79F6"}
	Cream        = lipgloss.AdaptiveColor{Dark: "#FFFDF5", Light: "#FFFDF5"}
	YellowGreen  = lipgloss.AdaptiveColor{Dark: "#ECFD65", Light: "#04B575"}
	Fuschia      = lipgloss.AdaptiveColor{Dark: "#EE6FF8", Light: "#EE6FF8"}
	Green        = lipgloss.AdaptiveColor{Dark: "#04B575", Light: "#04B575"}
	Red          = lipgloss.AdaptiveColor{Dark: "#ED567A", Light: "#FF4672"}
	FaintRed     = lipgloss.AdaptiveColor{Dark: "#C74665", Light: "#FF6F91"}
	NoColor      = lipgloss.AdaptiveColor{Dark: "", Light: ""}
)

var (
	TitleStyle    = lipgloss.NewStyle().Foreground(Cream).Background(Indigo)
	SubtitleStyle = lipgloss.NewStyle().Foreground(Cream).Background(SubtleIndigo)
	QuitMsgStyle  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#DDDADA", Dark: "#3C3C3C"})
	ErrorMsgStyle = lipgloss.NewStyle().Foreground(Red)
)
