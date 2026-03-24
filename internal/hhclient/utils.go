package hhclient

import (
	"fmt"
	"math/rand"
	"strings"
	"time"
)

func GenerateAndroidUserAgent() string {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	devices := strings.Split("23053RN02A, 23053RN02Y, 23053RN02I, 23053RN02L, 23077RABDC", ", ")
	device := devices[rnd.Intn(len(devices))]
	minor := rnd.Intn(51) + 100     // 100-150
	patch := rnd.Intn(5001) + 10000 // 10000-15000
	android := rnd.Intn(5) + 11     // 11-15

	return fmt.Sprintf("ru.hh.android/7.%d.%d, Device: %s, Android OS: %d",
		minor, patch, device, android)
}
