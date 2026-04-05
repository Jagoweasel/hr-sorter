package hh

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AndroidUserAgentGenerator implements the UserAgentGenerator interface for HH Android app.
type AndroidUserAgentGenerator struct {
	rnd *rand.Rand
}

// NewAndroidUserAgentGenerator creates a new instance of AndroidUserAgentGenerator.
func NewAndroidUserAgentGenerator() UserAgentGenerator {
	return &AndroidUserAgentGenerator{
		rnd: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Generate returns a randomized but valid HeadHunter Android User-Agent.
func (g *AndroidUserAgentGenerator) Generate() string {
	if g.rnd == nil {
		g.rnd = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	devices := strings.Split("23053RN02A, 23053RN02Y, 23053RN02I, 23053RN02L, 23077RABDC", ", ")
	device := devices[g.rnd.Intn(len(devices))]
	minor := g.rnd.Intn(51) + 100     // 100-150
	patch := g.rnd.Intn(5001) + 10000 // 10000-15000
	android := g.rnd.Intn(5) + 11     // 11-15

	return fmt.Sprintf("ru.hh.android/7.%d.%d, Device: %s, Android OS: %d (UUID: %s)",
		minor, patch, device, android, uuid.New().String())
}
