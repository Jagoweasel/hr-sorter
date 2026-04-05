package hh

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAndroidUserAgentGenerator_Generate(t *testing.T) {
	generator := &AndroidUserAgentGenerator{}
	ua := generator.Generate()

	// Format: ru.hh.android/7.<minor>.<patch>, Device: <model>, Android OS: <version> (UUID: <uuid4>)
	re := regexp.MustCompile(`^ru\.hh\.android/7\.\d+\.\d+, Device: .+, Android OS: \d+ \(UUID: [a-fA-F0-9]{8}-[a-fA-F0-9]{4}-4[a-fA-F0-9]{3}-[89abAB][a-fA-F0-9]{3}-[a-fA-F0-9]{12}\)$`)
	assert.Regexp(t, re, ua)

	// Ensure randomness
	ua2 := generator.Generate()
	assert.NotEqual(t, ua, ua2)
}
