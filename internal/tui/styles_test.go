package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestPaletteFor(t *testing.T) {
	t.Parallel()
	def, name := paletteFor("default")
	if name != "default" {
		t.Fatalf("name = %q", name)
	}
	dim, _ := paletteFor("dim")
	hc, _ := paletteFor("high-contrast")
	if def.accent == dim.accent || def.accent == hc.accent || dim.accent == hc.accent {
		t.Fatalf("accents should differ: default=%v dim=%v hc=%v", def.accent, dim.accent, hc.accent)
	}
	if def.selected == lipgloss.Color("236") {
		t.Fatal("default selected 236 is too faint")
	}
	unknown, resolved := paletteFor("rainbow")
	if resolved != "default" || unknown.accent != def.accent {
		t.Fatalf("unknown -> default, got %q %v", resolved, unknown.accent)
	}
}

func TestEveryThemeHasPalette(t *testing.T) {
	t.Parallel()
	var names []string
	for _, def := range settingDefs {
		if def.key == "theme" {
			names = def.values
			break
		}
	}
	if len(names) < 6 {
		t.Fatalf("theme list too short: %v", names)
	}
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			t.Fatalf("duplicate theme %q", name)
		}
		seen[name] = true
		_, got := paletteFor(name)
		if got != name {
			t.Fatalf("theme %q has no palette (resolved %q)", name, got)
		}
	}
}

func TestApplyTheme(t *testing.T) {
	t.Cleanup(func() { applyTheme("default") })

	def, _ := paletteFor("default")
	dim, _ := paletteFor("dim")
	hc, _ := paletteFor("high-contrast")

	applyTheme("dim")
	if appliedTheme != "dim" {
		t.Fatalf("applied = %q", appliedTheme)
	}
	if colorAccent != dim.accent {
		t.Fatalf("dim accent %v want %v", colorAccent, dim.accent)
	}
	if styleHandle.GetForeground() != colorAccent {
		t.Fatal("styleHandle not rebuilt for dim")
	}
	if styleSelected.GetBackground() != colorSelected {
		t.Fatal("styleSelected not rebuilt for dim")
	}
	if colorAccent == def.accent {
		t.Fatal("dim should not keep default accent")
	}

	applyTheme("high-contrast")
	if appliedTheme != "high-contrast" {
		t.Fatalf("applied = %q", appliedTheme)
	}
	if colorAccent != hc.accent {
		t.Fatalf("hc accent %v want %v", colorAccent, hc.accent)
	}
	if styleHandle.GetForeground() != hc.accent {
		t.Fatal("styleHandle not rebuilt for high-contrast")
	}

	applyTheme("rainbow")
	if appliedTheme != "default" {
		t.Fatalf("unknown applied = %q", appliedTheme)
	}
	if colorAccent != def.accent {
		t.Fatalf("fallback accent %v want %v", colorAccent, def.accent)
	}
}
