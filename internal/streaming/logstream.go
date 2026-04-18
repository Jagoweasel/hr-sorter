package streaming

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LogBroadcaster captures zap logger output and sends to subscribers
type LogBroadcaster struct {
	mu          sync.RWMutex
	subscribers map[chan string]struct{}
}

func NewLogBroadcaster() *LogBroadcaster {
	return &LogBroadcaster{
		subscribers: make(map[chan string]struct{}),
	}
}

func (b *LogBroadcaster) Stream(ctx context.Context, writer io.Writer) error {
	flusher, ok := writer.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	ch := make(chan string, 500)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
		close(ch)
	}()

	// Send initial comment to establish connection immediately in browser
	_, _ = fmt.Fprintf(writer, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			// Keep-alive ping
			_, err := fmt.Fprintf(writer, ": ping\n\n")
			if err != nil {
				return err
			}
			flusher.Flush()
		case entry := <-ch:
			// Clean up entry
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}

			// Split multiline logs and remove carriage returns
			entry = strings.ReplaceAll(entry, "\r", "")
			lines := strings.Split(entry, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				_, err := fmt.Fprintf(writer, "data: %s\n", line)
				if err != nil {
					return err
				}
			}
			_, err := fmt.Fprintf(writer, "\n")
			if err != nil {
				return err
			}
			flusher.Flush()
		}
	}
}

func (b *LogBroadcaster) Broadcast(entry string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subscribers {
		select {
		case ch <- entry:
		default:
			// Buffer full, skip to avoid blocking the logger
		}
	}
}

// Write implements io.Writer to be used as a zap core output
func (b *LogBroadcaster) Write(p []byte) (n int, err error) {
	b.Broadcast(string(p))
	return len(p), nil
}
