package ui

import "github.com/charmbracelet/lipgloss"

// Tokyo Night-ish palette, at home next to a typical dark terminal setup.
var (
	cText  = lipgloss.Color("#c0caf5")
	cBody  = lipgloss.Color("#c0caf5")
	cDim   = lipgloss.Color("#565f89")
	cMute  = lipgloss.Color("#a9b1d6")
	cWool  = lipgloss.Color("#c4b5a0")
	cCyan  = lipgloss.Color("#7dcfff")
	cBlue  = lipgloss.Color("#7aa2f7")
	cGood  = lipgloss.Color("#9ece6a")
	cWarn  = lipgloss.Color("#e0af68")
	cHot   = lipgloss.Color("#ff9e64")
	cBad   = lipgloss.Color("#f7768e")
	cAcc   = lipgloss.Color("#bb9af7")
	cLine  = lipgloss.Color("#3b4261")
	cFaint = lipgloss.Color("#24283b")
	cSelB  = lipgloss.Color("#292e42")
	// low phase of the blocked "bleat" breathing border
	cBadDim = lipgloss.Color("#7a3b4a")

	stText = lipgloss.NewStyle().Foreground(cText)
	stBody = lipgloss.NewStyle().Foreground(cBody)
	stDim  = lipgloss.NewStyle().Foreground(cDim)
	stMute = lipgloss.NewStyle().Foreground(cMute)
	stWool = lipgloss.NewStyle().Foreground(cWool)
	stHead = lipgloss.NewStyle().Foreground(cBlue)
	// Match herdr sidebar dots: red blocked, yellow working, blue done, green idle.
	stWork  = lipgloss.NewStyle().Foreground(cWarn).Bold(true)
	stBlck  = lipgloss.NewStyle().Foreground(cBad).Bold(true)
	stDone  = lipgloss.NewStyle().Foreground(cBlue)
	stIdle  = lipgloss.NewStyle().Foreground(cGood)
	stErr   = lipgloss.NewStyle().Foreground(cBad)
	stBar   = lipgloss.NewStyle().Foreground(cDim)
	stSel   = lipgloss.NewStyle().Foreground(cCyan).Bold(true)
	stKey   = lipgloss.NewStyle().Foreground(cCyan)
	stLine  = lipgloss.NewStyle().Foreground(cLine)
	stSelR  = lipgloss.NewStyle().Foreground(cText).Background(cSelB)
	stBrand = lipgloss.NewStyle().Foreground(cCyan).Bold(true)
	// soft drop shadow under the selected card: half blocks in a near-bg tone
	stShadow = lipgloss.NewStyle().Foreground(cFaint)
	// title chips: dark status-tinted background, xiaohongshu cover feel
	stChipBlck = lipgloss.NewStyle().Foreground(cBad).Background(lipgloss.Color("#402734")).Bold(true)
	stChipWork = lipgloss.NewStyle().Foreground(cWarn).Background(lipgloss.Color("#3d3223"))
	stChipDone = lipgloss.NewStyle().Foreground(cBlue).Background(lipgloss.Color("#24304a"))
	stChipIdle = lipgloss.NewStyle().Foreground(cGood).Background(lipgloss.Color("#2a3627"))
)
