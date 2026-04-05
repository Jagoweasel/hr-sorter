package streaming

import (
	"context"
	"io"
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
	// Create a new channel for this subscriber
	// Start an SSE/WS loop to send messages to writer
	panic("implement me with streaming loop")
}

func (b *LogBroadcaster) Broadcast(entry string) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	// Send entry to all subscriber channels
	panic("implement me with fan-out logic")
}

// Write implements io.Writer to be used as a zap core output
func (b *LogBroadcaster) Write(p []byte) (n int, err error) {
	b.Broadcast(string(p))
	return len(p), nil
}
