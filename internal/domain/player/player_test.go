package player

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestPickColourPrefersUnused(t *testing.T) {
	used := []string{Palette[0], Palette[1]}
	got := PickColour(used)
	if got == Palette[0] || got == Palette[1] {
		t.Errorf("PickColour returned an in-use colour %q", got)
	}
	if got != Palette[2] {
		t.Errorf("PickColour = %q, want the first free palette entry %q", got, Palette[2])
	}
}

func TestPickColourIsCaseInsensitive(t *testing.T) {
	used := []string{strings.ToLower(Palette[0])}
	if got := PickColour(used); got == Palette[0] {
		t.Errorf("PickColour ignored a lower-cased in-use colour: %q", got)
	}
}

func TestPickColourFallsBackToLeastUsed(t *testing.T) {
	// Every colour used once, and the first used twice more.
	var used []string
	used = append(used, Palette...)
	used = append(used, Palette[0], Palette[0])
	// Make the third colour the clear minimum by adding extras elsewhere.
	for i, c := range Palette {
		if i != 2 {
			used = append(used, c)
		}
	}
	if got := PickColour(used); got != Palette[2] {
		t.Errorf("PickColour = %q, want the least-used %q", got, Palette[2])
	}
}

func TestPickColourWithNoPlayers(t *testing.T) {
	if got := PickColour(nil); got != Palette[0] {
		t.Errorf("PickColour(nil) = %q, want %q", got, Palette[0])
	}
}

func TestPaletteIsValidAndDistinct(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Palette {
		got, err := ValidateColour(c)
		if err != nil {
			t.Errorf("palette entry %q is not a valid colour: %v", c, err)
		}
		if seen[got] {
			t.Errorf("palette contains %q twice", got)
		}
		seen[got] = true
	}
	if len(Palette) < 8 {
		t.Errorf("palette has only %d colours; too few for an office", len(Palette))
	}
}

func TestValidateName(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr error
	}{
		{"Alice", "Alice", nil},
		{"  Bob  ", "Bob", nil},
		{"Bob   Smith", "Bob Smith", nil},
		{"", "", ErrEmptyName},
		{"   ", "", ErrEmptyName},
		{"\t\n", "", ErrEmptyName},
		{strings.Repeat("a", MaxNameLength), strings.Repeat("a", MaxNameLength), nil},
		{strings.Repeat("a", MaxNameLength+1), "", ErrNameTooLong},
	}
	for _, tc := range cases {
		got, err := ValidateName(tc.in)
		if tc.wantErr != nil {
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("ValidateName(%q) error = %v, want %v", tc.in, err, tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Errorf("ValidateName(%q) unexpected error %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ValidateName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateNameCountsRunesNotBytes(t *testing.T) {
	// Twenty-four accented characters is 24 runes but 48 bytes.
	name := strings.Repeat("é", MaxNameLength)
	if _, err := ValidateName(name); err != nil {
		t.Errorf("a %d-rune name should be accepted: %v", MaxNameLength, err)
	}
}

func TestValidateColour(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"#ff5f87", "#FF5F87", false},
		{"#FF5F87", "#FF5F87", false},
		{" #ff5f87 ", "#FF5F87", false},
		{"ff5f87", "", true},
		{"#FF5F8", "", true},
		{"#GGGGGG", "", true},
		{"", "", true},
		{"red", "", true},
	}
	for _, tc := range cases {
		got, err := ValidateColour(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ValidateColour(%q) should have failed", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ValidateColour(%q) unexpected error %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ValidateColour(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSafeColourFallsBack(t *testing.T) {
	if got := SafeColour("not a colour"); got != DefaultColour {
		t.Errorf("SafeColour(bad) = %q, want %q", got, DefaultColour)
	}
	if got := SafeColour("#abcdef"); got != "#ABCDEF" {
		t.Errorf("SafeColour(good) = %q, want %q", got, "#ABCDEF")
	}
}

func TestArchived(t *testing.T) {
	var p Player
	if p.Archived() {
		t.Error("a new player should not be archived")
	}
	// Archiving is a timestamp rather than a deletion, so history survives.
	now := time.Now()
	p.ArchivedAt = &now
	if !p.Archived() {
		t.Error("player with ArchivedAt should report archived")
	}
}
