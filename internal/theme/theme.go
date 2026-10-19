// Package theme defines Outfitter's semantic color tokens and named palettes.
package theme

import (
	"image/color"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// Mode identifies a palette and accessibility mode.
type Mode string

// Supported visual modes.
const (
	ModeDark  Mode = "midnight"
	ModeLight Mode = "daylight"
	ModeMono  Mode = "monochrome"
)

// Palette is the set of semantic color tokens used by the interface. Values are
// empty when a token should inherit the terminal's default color.
type Palette struct {
	Canvas  string
	Surface string
	Border  string
	Text    string
	Muted   string
	Accent  string
	Good    string
	Low     string
	Error   string
	Stale   string
}

// Midnight returns the dark palette from the TUI specification.
func Midnight() Palette {
	return Palette{
		Canvas:  "#0b1020",
		Surface: "#151d2c",
		Border:  "#344155",
		Text:    "#e6edf3",
		Muted:   "#a6b2c3",
		Accent:  "#7dd3fc",
		Good:    "#86efac",
		Low:     "#fcd34d",
		Error:   "#fda4af",
		Stale:   "#c4b5fd",
	}
}

// Daylight returns the light palette from the TUI specification.
func Daylight() Palette {
	return Palette{
		Canvas:  "#f8fafc",
		Surface: "#ffffff",
		Border:  "#cbd5e1",
		Text:    "#0f172a",
		Muted:   "#475569",
		Accent:  "#0369a1",
		Good:    "#166534",
		Low:     "#92400e",
		Error:   "#be123c",
		Stale:   "#6d28d9",
	}
}

// Monochrome returns a true grayscale palette. Combined with the marker glyphs
// it keeps every state distinguishable without chromatic color.
func Monochrome() Palette {
	return Palette{
		Canvas:  "#000000",
		Surface: "#111111",
		Border:  "#888888",
		Text:    "#ffffff",
		Muted:   "#cccccc",
		Accent:  "#ffffff",
		Good:    "#f0f0f0",
		Low:     "#ffffff",
		Error:   "#ffffff",
		Stale:   "#dddddd",
	}
}

// ResolveMode normalizes a requested mode and honors NO_COLOR. An empty or
// unknown mode resolves to Midnight.
func ResolveMode(mode Mode) Mode {
	if os.Getenv("NO_COLOR") != "" {
		return ModeMono
	}
	switch mode {
	case ModeDark, ModeLight, ModeMono:
		return mode
	default:
		return ModeDark
	}
}

// PaletteFor returns the palette for a mode. Unknown modes resolve to Midnight.
func PaletteFor(mode Mode) Palette {
	switch mode {
	case ModeLight:
		return Daylight()
	case ModeMono:
		return Monochrome()
	default:
		return Midnight()
	}
}

// Styles contains semantic Lip Gloss styles derived from a palette.
type Styles struct {
	Palette Palette

	CanvasColor  color.Color
	SurfaceColor color.Color
	BorderColor  color.Color

	Canvas  lipgloss.Style
	Surface lipgloss.Style
	Border  lipgloss.Style
	Text    lipgloss.Style
	Muted   lipgloss.Style
	Accent  lipgloss.Style
	Good    lipgloss.Style
	Low     lipgloss.Style
	Error   lipgloss.Style
	Stale   lipgloss.Style
}

// NewStyles builds the semantic style set for a mode.
func NewStyles(mode Mode) Styles {
	mode = ResolveMode(mode)
	palette := PaletteFor(mode)
	styles := Styles{
		Palette:      palette,
		CanvasColor:  tokenColor(palette.Canvas),
		SurfaceColor: tokenColor(palette.Surface),
		BorderColor:  tokenColor(palette.Border),
		Canvas:       lipgloss.NewStyle().Foreground(tokenColor(palette.Text)).Background(tokenColor(palette.Canvas)),
		Surface:      lipgloss.NewStyle().Foreground(tokenColor(palette.Text)).Background(tokenColor(palette.Surface)),
		Border:       lipgloss.NewStyle().Foreground(tokenColor(palette.Border)),
		Text:         lipgloss.NewStyle().Foreground(tokenColor(palette.Text)),
		Muted:        lipgloss.NewStyle().Foreground(tokenColor(palette.Muted)),
		Accent:       lipgloss.NewStyle().Foreground(tokenColor(palette.Accent)),
		Good:         lipgloss.NewStyle().Foreground(tokenColor(palette.Good)),
		Low:          lipgloss.NewStyle().Foreground(tokenColor(palette.Low)),
		Error:        lipgloss.NewStyle().Foreground(tokenColor(palette.Error)),
		Stale:        lipgloss.NewStyle().Foreground(tokenColor(palette.Stale)),
	}
	if mode == ModeMono {
		// Structural emphasis preserves the hierarchy when color is absent.
		styles.Accent = styles.Accent.Bold(true)
		styles.Good = styles.Good.Bold(true)
		styles.Low = styles.Low.Bold(true)
		styles.Error = styles.Error.Bold(true)
	}
	return styles
}

// ByToken maps a semantic token name (as returned by domain.Marker) to a style.
func (s Styles) ByToken(token string) lipgloss.Style {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "canvas":
		return s.Canvas
	case "surface":
		return s.Surface
	case "border":
		return s.Border
	case "muted":
		return s.Muted
	case "accent":
		return s.Accent
	case "good":
		return s.Good
	case "low":
		return s.Low
	case "error":
		return s.Error
	case "stale":
		return s.Stale
	case "text", "":
		return s.Text
	default:
		return s.Text
	}
}

// ColorFor returns the terminal color for a semantic token name.
func (s Styles) ColorFor(token string) color.Color {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "canvas":
		return s.CanvasColor
	case "surface":
		return s.SurfaceColor
	case "border":
		return s.BorderColor
	case "muted":
		return tokenColor(s.Palette.Muted)
	case "accent":
		return tokenColor(s.Palette.Accent)
	case "good":
		return tokenColor(s.Palette.Good)
	case "low":
		return tokenColor(s.Palette.Low)
	case "error":
		return tokenColor(s.Palette.Error)
	case "stale":
		return tokenColor(s.Palette.Stale)
	case "text", "":
		return tokenColor(s.Palette.Text)
	default:
		return tokenColor(s.Palette.Text)
	}
}

// WithBackground returns a token style with the given background token.
func (s Styles) WithBackground(token, background string) lipgloss.Style {
	style := s.ByToken(token)
	if background == "" {
		return style
	}
	return style.Background(s.ColorFor(background))
}

func tokenColor(value string) color.Color {
	if value == "" {
		return lipgloss.NoColor{}
	}
	return lipgloss.Color(value)
}
