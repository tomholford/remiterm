package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type palette struct {
	dim, accent, own, err, ok, media, reply, selected color.Color
}

var palettes = map[string]palette{
	"default": {
		dim:      lipgloss.Color("245"),
		accent:   lipgloss.Color("212"),
		own:      lipgloss.Color("117"),
		err:      lipgloss.Color("203"),
		ok:       lipgloss.Color("114"),
		media:    lipgloss.Color("178"),
		reply:    lipgloss.Color("244"),
		selected: lipgloss.Color("240"), // 236 was too close to default bg
	},
	"dim": {
		dim:      lipgloss.Color("242"),
		accent:   lipgloss.Color("175"),
		own:      lipgloss.Color("109"),
		err:      lipgloss.Color("167"),
		ok:       lipgloss.Color("108"),
		media:    lipgloss.Color("144"),
		reply:    lipgloss.Color("241"),
		selected: lipgloss.Color("238"),
	},
	"high-contrast": {
		dim:      lipgloss.Color("7"),
		accent:   lipgloss.Color("15"),
		own:      lipgloss.Color("14"),
		err:      lipgloss.Color("9"),
		ok:       lipgloss.Color("10"),
		media:    lipgloss.Color("11"),
		reply:    lipgloss.Color("7"),
		selected: lipgloss.Color("4"),
	},
	// Classic Monokai (Hazenberg, 2006 / Sublime default).
	"monokai": {
		dim:      lipgloss.Color("#75715E"),
		accent:   lipgloss.Color("#F92672"),
		own:      lipgloss.Color("#66D9EF"),
		err:      lipgloss.Color("#F92672"),
		ok:       lipgloss.Color("#A6E22E"),
		media:    lipgloss.Color("#E6DB74"),
		reply:    lipgloss.Color("#75715E"),
		selected: lipgloss.Color("#49483E"),
	},
	// Tomorrow Night (Chris Kempson).
	"tomorrow-night": {
		dim:      lipgloss.Color("#969896"),
		accent:   lipgloss.Color("#81A2BE"),
		own:      lipgloss.Color("#8ABEB7"),
		err:      lipgloss.Color("#CC6666"),
		ok:       lipgloss.Color("#B5BD68"),
		media:    lipgloss.Color("#F0C674"),
		reply:    lipgloss.Color("#969896"),
		selected: lipgloss.Color("#373B41"),
	},
	// Dracula Classic (https://spec.draculatheme.com).
	"dracula": {
		dim:      lipgloss.Color("#6272A4"),
		accent:   lipgloss.Color("#FF79C6"),
		own:      lipgloss.Color("#8BE9FD"),
		err:      lipgloss.Color("#FF5555"),
		ok:       lipgloss.Color("#50FA7B"),
		media:    lipgloss.Color("#F1FA8C"),
		reply:    lipgloss.Color("#6272A4"),
		selected: lipgloss.Color("#44475A"),
	},
}

func paletteFor(name string) (palette, string) {
	if p, ok := palettes[name]; ok {
		return p, name
	}
	return palettes["default"], "default"
}

var (
	appliedTheme string

	colorDim      color.Color
	colorAccent   color.Color
	colorOwn      color.Color
	colorErr      color.Color
	colorOK       color.Color
	colorMedia    color.Color
	colorReply    color.Color
	colorSelected color.Color

	styleHeader         lipgloss.Style
	styleStatus         lipgloss.Style
	styleErr            lipgloss.Style
	styleHandle         lipgloss.Style
	styleHandleOwn      lipgloss.Style
	styleTime           lipgloss.Style
	styleReply          lipgloss.Style
	styleMedia          lipgloss.Style
	styleSelected       lipgloss.Style
	styleHelp           lipgloss.Style
	styleBanner         lipgloss.Style
	styleFooter         lipgloss.Style
	styleConnOnline     lipgloss.Style
	styleConnConnecting lipgloss.Style
	styleConnOffline    lipgloss.Style
	styleProfileCard    lipgloss.Style
	styleProfileTitle   lipgloss.Style
	styleProfileName    lipgloss.Style
	styleProfileLabel   lipgloss.Style
	styleProfileBadge   lipgloss.Style
	styleProfileStatus  lipgloss.Style
)

func init() {
	applyTheme("default")
}

// applyTheme rebuilds package color tokens and styles. Unknown names fall
// back to default. Render call sites keep using styleHandle etc.
func applyTheme(name string) {
	p, resolved := paletteFor(name)
	appliedTheme = resolved
	colorDim = p.dim
	colorAccent = p.accent
	colorOwn = p.own
	colorErr = p.err
	colorOK = p.ok
	colorMedia = p.media
	colorReply = p.reply
	colorSelected = p.selected
	rebuildStyles()
}

func rebuildStyles() {
	styleHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent).
		Padding(0, 1)

	styleStatus = lipgloss.NewStyle().
		Foreground(colorDim).
		Padding(0, 1)

	styleErr = lipgloss.NewStyle().
		Foreground(colorErr).
		Padding(0, 1)

	styleHandle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent)

	styleHandleOwn = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorOwn)

	styleTime = lipgloss.NewStyle().
		Foreground(colorDim)

	styleReply = lipgloss.NewStyle().
		Foreground(colorReply).
		Italic(true)

	styleMedia = lipgloss.NewStyle().
		Foreground(colorMedia)

	styleSelected = lipgloss.NewStyle().
		Background(colorSelected).
		Padding(0, 1)

	styleHelp = lipgloss.NewStyle().
		Foreground(colorDim).
		Padding(1, 2)

	styleBanner = lipgloss.NewStyle().
		Foreground(colorAccent).
		Padding(0, 1)

	styleFooter = lipgloss.NewStyle().
		Foreground(colorDim).
		Faint(true).
		Padding(0, 1)

	styleConnOnline = lipgloss.NewStyle().
		Foreground(colorOK).
		Faint(true).
		Padding(0, 1)

	styleConnConnecting = lipgloss.NewStyle().
		Foreground(colorDim).
		Faint(true).
		Padding(0, 1)

	styleConnOffline = lipgloss.NewStyle().
		Foreground(colorErr).
		Faint(true).
		Padding(0, 1)

	styleProfileCard = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2)

	styleProfileTitle = lipgloss.NewStyle().
		Bold(true).
		Foreground(colorAccent)

	styleProfileName = lipgloss.NewStyle().
		Bold(true)

	styleProfileLabel = lipgloss.NewStyle().
		Foreground(colorDim)

	styleProfileBadge = lipgloss.NewStyle().
		Foreground(colorOwn).
		Bold(true)

	styleProfileStatus = lipgloss.NewStyle().
		Foreground(colorOK)
}
