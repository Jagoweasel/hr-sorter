package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"sync"
)

//go:embed locales/*.json
var localesAssets embed.FS

// LocalizationService handles English and Russian translations
type LocalizationService struct {
	mu           sync.RWMutex
	translations map[string]map[string]string // locale -> key -> value
}

func NewLocalizationService() (*LocalizationService, error) {
	ls := &LocalizationService{
		translations: make(map[string]map[string]string),
	}
	if err := ls.Load(); err != nil {
		return nil, fmt.Errorf("failed to load translations: %w", err)
	}
	return ls, nil
}

func (s *LocalizationService) Load(locales ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := fs.ReadDir(localesAssets, "locales")
	if err != nil {
		return fmt.Errorf("failed to read locales dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		if !strings.HasSuffix(fileName, ".json") {
			continue
		}

		locale := strings.TrimSuffix(fileName, ".json")

		// If specific locales are requested, skip if not in the list
		if len(locales) > 0 {
			found := false
			for _, l := range locales {
				if l == locale {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}

		content, err := fs.ReadFile(localesAssets, path.Join("locales", fileName))
		if err != nil {
			return fmt.Errorf("failed to read locale file %s: %w", fileName, err)
		}

		var m map[string]string
		if err := json.Unmarshal(content, &m); err != nil {
			return fmt.Errorf("failed to unmarshal locale file %s: %w", fileName, err)
		}

		s.translations[locale] = m
	}

	return nil
}

func (s *LocalizationService) Translate(key string, locale string, args ...interface{}) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	trans, ok := s.translations[locale]
	if !ok {
		// Fallback to "en" if requested locale is not found
		trans, ok = s.translations["en"]
	}

	if ok {
		if val, exists := trans[key]; exists {
			if len(args) > 0 {
				return fmt.Sprintf(val, args...)
			}
			return val
		}
	}

	// If key not found, return key itself as a fallback
	if len(args) > 0 {
		return fmt.Sprintf(key, args...)
	}
	return key
}

func (s *LocalizationService) Tr(key string, locale string) string {
	return s.Translate(key, locale)
}
