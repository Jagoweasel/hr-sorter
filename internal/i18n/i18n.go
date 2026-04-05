package i18n

import (
	"embed"
	"fmt"
	"sync"
)

//go:embed locales/*.json
var localesAssets embed.FS

// LocalizationService handles English and Russian translations
type LocalizationService struct {
	mu           sync.RWMutex
	translations map[string]map[string]string // locale -> key -> value
}

func NewLocalizationService() *LocalizationService {
	return &LocalizationService{
		translations: make(map[string]map[string]string),
	}
}

func (s *LocalizationService) Load(locales ...string) error {
	// Parse JSON from embedded localesAssets
	panic("implement me with go:embed loading")
}

func (s *LocalizationService) Translate(key string, locale string, args ...interface{}) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Default to English if locale not found
	// Use fmt.Sprintf for args
	res := fmt.Sprintf(key, args...)
	return res
}

func (s *LocalizationService) Tr(key string, locale string) string {
	return s.Translate(key, locale)
}
