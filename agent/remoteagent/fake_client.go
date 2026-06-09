package remoteagent

import (
	"fmt"
	"sync"
)

// FakeA2AClientConfig configures a FakeA2AClient's behavior.
type FakeA2AClientConfig struct {
	// Card is the agent card returned by this client.
	Card AgentCard

	// Events is the pre-defined list of stream events to return.
	// They are emitted in order, one at a time, into the stream channel.
	Events []StreamEvent

	// SendDelay simulates network latency per event (0 = instant).
	// Not implemented in this teaching model; reserved for future use.
	// SendDelay time.Duration

	// CancelError is the error returned by CancelTask.
	// If nil, CancelTask succeeds silently.
	CancelError error

	// DestroyError is the error returned by Destroy.
	DestroyError error

	// MaxCancels is the maximum number of CancelTask calls allowed.
	// If exceeded, CancelTask returns an error. Zero means unlimited.
	MaxCancels int

	// RecordCancels records every taskID passed to CancelTask for assertions.
	// When nil, cancels are not recorded.
	RecordCancels *[]string
}

// FakeA2AClient is an in-memory A2AClient implementation for testing.
//
// It returns a pre-configured stream of events from SendStreamingMessage.
// CancelTask and Destroy are controllable via FakeA2AClientConfig.
//
// Thread-safe.
type FakeA2AClient struct {
	mu       sync.Mutex
	card     AgentCard
	events   []StreamEvent
	canceled map[string]bool

	cancelError  error
	destroyError error
	maxCancels   int
	cancelCount  int
	recordCancels *[]string

	destroyed bool
}

// NewFakeA2AClient creates a new FakeA2AClient.
func NewFakeA2AClient(cfg FakeA2AClientConfig) *FakeA2AClient {
	return &FakeA2AClient{
		card:          cfg.Card,
		events:        cfg.Events,
		canceled:      make(map[string]bool),
		cancelError:   cfg.CancelError,
		destroyError:  cfg.DestroyError,
		maxCancels:    cfg.MaxCancels,
		recordCancels: cfg.RecordCancels,
	}
}

// AgentCard returns the pre-configured agent card.
func (c *FakeA2AClient) AgentCard() AgentCard {
	return c.card
}

// SendStreamingMessage returns a channel that emits the pre-configured events
// in order, then closes.
func (c *FakeA2AClient) SendStreamingMessage(req SendMessageRequest) <-chan StreamEvent {
	ch := make(chan StreamEvent, len(c.events)+1)

	go func() {
		c.mu.Lock()
		if c.destroyed {
			c.mu.Unlock()
			ch <- StreamEvent{
				Err: fmt.Errorf("remoteagent: client is destroyed"),
			}
			close(ch)
			return
		}
		c.mu.Unlock()

		for _, ev := range c.events {
			ch <- ev
		}
		close(ch)
	}()

	return ch
}

// CancelTask marks a remote task as canceled.
//
// If CancelError is set, it is returned. If MaxCancels is set and the cancel
// count is exceeded, an error is returned. If RecordCancels is non-nil, the
// taskID is appended.
func (c *FakeA2AClient) CancelTask(taskID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.destroyed {
		return fmt.Errorf("remoteagent: client is destroyed")
	}

	if c.cancelError != nil {
		return c.cancelError
	}

	if c.maxCancels > 0 && c.cancelCount >= c.maxCancels {
		return fmt.Errorf("remoteagent: max cancel count (%d) exceeded", c.maxCancels)
	}

	c.cancelCount++
	c.canceled[taskID] = true

	if c.recordCancels != nil {
		*c.recordCancels = append(*c.recordCancels, taskID)
	}

	return nil
}

// Destroy marks the client as destroyed. Subsequent calls return an error.
func (c *FakeA2AClient) Destroy() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.destroyError != nil {
		return c.destroyError
	}

	c.destroyed = true
	return nil
}

// IsCanceled returns whether a task was canceled via this client.
func (c *FakeA2AClient) IsCanceled(taskID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.canceled[taskID]
}

// CancelCount returns the number of times CancelTask was called.
func (c *FakeA2AClient) CancelCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cancelCount
}

// IsDestroyed returns whether Destroy was called.
func (c *FakeA2AClient) IsDestroyed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.destroyed
}
