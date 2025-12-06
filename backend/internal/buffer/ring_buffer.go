// Package buffer provides ring buffer implementation for session output caching.
package buffer

import (
	"sync"
)

// RingBuffer is a thread-safe circular buffer that stores the most recent data
// up to a specified capacity. When the buffer is full, oldest data is discarded
// to make room for new data.
//
// This is used to cache PTY output for hot restore functionality, allowing
// clients to receive recent terminal history when reconnecting.
type RingBuffer struct {
	data     []byte
	capacity int
	mu       sync.RWMutex
}

// NewRingBuffer creates a new RingBuffer with the specified capacity.
// The capacity must be greater than 0; if not, it defaults to 1.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = 1
	}
	return &RingBuffer{
		data:     make([]byte, 0, capacity),
		capacity: capacity,
	}
}

// Write appends data to the buffer. If the total data exceeds capacity,
// the oldest data is discarded to make room for new data.
// This method implements io.Writer interface.
func (rb *RingBuffer) Write(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	// If incoming data is larger than capacity, only keep the last 'capacity' bytes
	if len(p) >= rb.capacity {
		rb.data = make([]byte, rb.capacity)
		copy(rb.data, p[len(p)-rb.capacity:])
		return len(p), nil
	}

	// Calculate how much space we need
	newLen := len(rb.data) + len(p)

	if newLen <= rb.capacity {
		// We have enough space, just append
		rb.data = append(rb.data, p...)
	} else {
		// Need to discard oldest data
		// Calculate how many bytes to discard
		discard := newLen - rb.capacity

		// Create new slice with remaining old data + new data
		newData := make([]byte, rb.capacity)
		copy(newData, rb.data[discard:])
		copy(newData[len(rb.data)-discard:], p)
		rb.data = newData
	}

	return len(p), nil
}

// ReadAll returns a copy of all data currently in the buffer.
// The returned slice is safe to use without holding the lock.
func (rb *RingBuffer) ReadAll() []byte {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if len(rb.data) == 0 {
		return nil
	}

	// Return a copy to avoid data races
	result := make([]byte, len(rb.data))
	copy(result, rb.data)
	return result
}

// Clear removes all data from the buffer.
func (rb *RingBuffer) Clear() {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	rb.data = rb.data[:0]
}

// Len returns the current number of bytes in the buffer.
func (rb *RingBuffer) Len() int {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	return len(rb.data)
}

// Cap returns the capacity of the buffer.
func (rb *RingBuffer) Cap() int {
	return rb.capacity
}
