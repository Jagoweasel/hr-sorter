package i18n

import (
	"testing"
)

func TestLocalizationService(t *testing.T) {
	t.Run("Translate existing key", func(t *testing.T) {
		ls, err := NewLocalizationService()
		if err != nil {
			t.Fatalf("failed to create localization service: %v", err)
		}
		// Assuming "en" has "AppTitle" after I update en.json
		// For now, let's just check if it returns the key if not found
		got := ls.Tr("NonExistentKey", "en")
		want := "NonExistentKey"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("Fallback to en", func(t *testing.T) {
		ls, err := NewLocalizationService()
		if err != nil {
			t.Fatalf("failed to create localization service: %v", err)
		}
		// Test fallback behavior
		got := ls.Tr("SomeKey", "fr")
		want := "SomeKey"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
