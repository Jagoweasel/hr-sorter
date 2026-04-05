package streaming

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
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

	ch := make(chan string, 100)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.subscribers, ch)
		b.mu.Unlock()
		close(ch)
	}()

	for {
		select {
		case <-ctx.Done():
			return nil
		case entry := <-ch:
			_, err := fmt.Fprintf(writer, "data: %s\n\n", entry)
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
