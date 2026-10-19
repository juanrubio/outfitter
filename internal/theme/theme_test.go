package theme_test

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/juanrubio/outfitter/internal/theme"
)

func TestPalettes(t *testing.T) {
	midnight := theme.Midnight()
	wantMidnight := theme.Palette{
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
	if midnight != wantMidnight {
		t.Fatalf("Midnight() = %#v, want %#v", midnight, wantMidnight)
	}

	daylight := theme.Daylight()
	wantDaylight := theme.Palette{
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
	if daylight != wantDaylight {
		t.Fatalf("Daylight() = %#v, want %#v", daylight, wantDaylight)
	}

	if theme.PaletteFor(theme.ModeDark) != midnight {
		t.Fatal("PaletteFor(ModeDark) did not return Midnight")
	}
	if theme.PaletteFor(theme.ModeLight) != daylight {
		t.Fatal("PaletteFor(ModeLight) did not return Daylight")
	}
	if theme.PaletteFor("unknown") != midnight {
		t.Fatal("unknown mode should resolve to Midnight")
	}
}

func TestMonochromeIsGrayscale(t *testing.T) {
	styles := theme.NewStyles(theme.ModeMono)
	values := map[string]string{
		"canvas":  styles.Palette.Canvas,
		"surface": styles.Palette.Surface,
		"border":  styles.Palette.Border,
		"text":    styles.Palette.Text,
		"muted":   styles.Palette.Muted,
		"accent":  styles.Palette.Accent,
		"good":    styles.Palette.Good,
		"low":     styles.Palette.Low,
		"error":   styles.Palette.Error,
		"stale":   styles.Palette.Stale,
	}
	for token, value := range values {
		if value == "" {
			t.Fatalf("monochrome token %q is empty", token)
		}
		red, green, blue, _ := styles.ColorFor(token).RGBA()
		if red != green || green != blue {
			t.Errorf("monochrome token %q is not grayscale: RGBA %d %d %d", token, red, green, blue)
		}
	}
}

func TestResolveMode(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	if got := theme.ResolveMode(theme.ModeDark); got != theme.ModeDark {
		t.Errorf("ResolveMode(ModeDark) = %q", got)
	}
	if got := theme.ResolveMode(theme.ModeLight); got != theme.ModeLight {
		t.Errorf("ResolveMode(ModeLight) = %q", got)
	}
	if got := theme.ResolveMode(theme.ModeMono); got != theme.ModeMono {
		t.Errorf("ResolveMode(ModeMono) = %q", got)
	}
	if got := theme.ResolveMode(""); got != theme.ModeDark {
		t.Errorf("ResolveMode(empty) = %q, want midnight", got)
	}
	if got := theme.ResolveMode("unknown"); got != theme.ModeDark {
		t.Errorf("ResolveMode(unknown) = %q, want midnight", got)
	}

	t.Setenv("NO_COLOR", "1")
	if got := theme.ResolveMode(theme.ModeDark); got != theme.ModeMono {
		t.Errorf("NO_COLOR ResolveMode(ModeDark) = %q, want monochrome", got)
	}
}

func TestByToken(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	styles := theme.NewStyles(theme.ModeLight)
	tokens := map[string]string{
		"accent": "#0369a1",
		"good":   "#166534",
		"low":    "#92400e",
		"error":  "#be123c",
		"muted":  "#475569",
		"stale":  "#6d28d9",
	}
	for token, hex := range tokens {
		gotR, gotG, gotB, gotA := styles.ByToken(token).GetForeground().RGBA()
		wantR, wantG, wantB, wantA := lipgloss.Color(hex).RGBA()
		if gotR != wantR || gotG != wantG || gotB != wantB || gotA != wantA {
			t.Errorf("token %q foreground = %d/%d/%d/%d, want %d/%d/%d/%d",
				token, gotR, gotG, gotB, gotA, wantR, wantG, wantB, wantA)
		}
	}

	if got := styles.ByToken("accent").GetForeground(); got == nil {
		t.Fatal("accent token has no foreground")
	}
	fallbackR, _, _, _ := styles.ByToken("missing").GetForeground().RGBA()
	textR, _, _, _ := styles.Text.GetForeground().RGBA()
	if fallbackR != textR {
		t.Fatal("unknown token should fall back to text")
	}
}
