package mapping

import (
	"context"
	"regexp"
	"sync"
)

// RegexMapper classifies vacancies into categories using regex rules
type RegexMapper struct {
	mu    sync.RWMutex
	rules map[string]*regexp.Regexp // category -> regex
}

func NewRegexMapper() *RegexMapper {
	return &RegexMapper{
		rules: make(map[string]*regexp.Regexp),
	}
}

func (m *RegexMapper) UpdateRules(ctx context.Context, rules map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Pre-compile regex rules
	// E.g. "Lead": regexp.MustCompile(".*Lead.*")
	panic("implement me with regex compilation")
}

func (m *RegexMapper) Classify(title string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Iterate through rules and find first match
	panic("implement me with regex matching logic")
}
