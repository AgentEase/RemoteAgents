package buffer

import (
	"bytes"
	"testing"
)

func TestNewRingBuffer(t *testing.T) {
	// Test with valid capacity
	rb := NewRingBuffer(100)
	if rb.Cap() != 100 {
		t.Errorf("expected capacity 100, got %d", rb.Cap())
	}
	if rb.Len() != 0 {
		t.Errorf("expected length 0, got %d", rb.Len())
	}

	// Test with zero capacity (should default to 1)
	rb = NewRingBuffer(0)
	if rb.Cap() != 1 {
		t.Errorf("expected capacity 1 for zero input, got %d", rb.Cap())
	}

	// Test with negative capacity (should default to 1)
	rb = NewRingBuffer(-5)
	if rb.Cap() != 1 {
		t.Errorf("expected capacity 1 for negative input, got %d", rb.Cap())
	}
}

func TestRingBuffer_Write(t *testing.T) {
	rb := NewRingBuffer(10)

	// Write data that fits
	n, err := rb.Write([]byte("hello"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected n=5, got %d", n)
	}
	if rb.Len() != 5 {
		t.Errorf("expected length 5, got %d", rb.Len())
	}

	// Write more data that still fits
	n, err = rb.Write([]byte("world"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected n=5, got %d", n)
	}
	if rb.Len() != 10 {
		t.Errorf("expected length 10, got %d", rb.Len())
	}

	data := rb.ReadAll()
	if !bytes.Equal(data, []byte("helloworld")) {
		t.Errorf("expected 'helloworld', got '%s'", string(data))
	}
}

func TestRingBuffer_WriteOverflow(t *testing.T) {
	rb := NewRingBuffer(10)

	// Fill the buffer
	rb.Write([]byte("0123456789"))

	// Write more data, should discard oldest
	rb.Write([]byte("abc"))

	data := rb.ReadAll()
	// Should have discarded "012" and kept "3456789abc"
	if !bytes.Equal(data, []byte("3456789abc")) {
		t.Errorf("expected '3456789abc', got '%s'", string(data))
	}
	if rb.Len() != 10 {
		t.Errorf("expected length 10, got %d", rb.Len())
	}
}

func TestRingBuffer_WriteLargerThanCapacity(t *testing.T) {
	rb := NewRingBuffer(5)

	// Write data larger than capacity
	n, err := rb.Write([]byte("0123456789"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 10 {
		t.Errorf("expected n=10, got %d", n)
	}

	data := rb.ReadAll()
	// Should only keep the last 5 bytes
	if !bytes.Equal(data, []byte("56789")) {
		t.Errorf("expected '56789', got '%s'", string(data))
	}
	if rb.Len() != 5 {
		t.Errorf("expected length 5, got %d", rb.Len())
	}
}

func TestRingBuffer_WriteEmpty(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("hello"))

	// Write empty data
	n, err := rb.Write([]byte{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 0 {
		t.Errorf("expected n=0, got %d", n)
	}

	// Buffer should be unchanged
	data := rb.ReadAll()
	if !bytes.Equal(data, []byte("hello")) {
		t.Errorf("expected 'hello', got '%s'", string(data))
	}
}

func TestRingBuffer_ReadAll(t *testing.T) {
	rb := NewRingBuffer(10)

	// ReadAll on empty buffer
	data := rb.ReadAll()
	if data != nil {
		t.Errorf("expected nil for empty buffer, got %v", data)
	}

	// Write and read
	rb.Write([]byte("test"))
	data = rb.ReadAll()
	if !bytes.Equal(data, []byte("test")) {
		t.Errorf("expected 'test', got '%s'", string(data))
	}

	// Verify ReadAll returns a copy (modifying returned slice shouldn't affect buffer)
	data[0] = 'X'
	data2 := rb.ReadAll()
	if !bytes.Equal(data2, []byte("test")) {
		t.Errorf("ReadAll should return a copy, got '%s'", string(data2))
	}
}

func TestRingBuffer_Clear(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("hello"))

	rb.Clear()

	if rb.Len() != 0 {
		t.Errorf("expected length 0 after clear, got %d", rb.Len())
	}

	data := rb.ReadAll()
	if data != nil {
		t.Errorf("expected nil after clear, got %v", data)
	}

	// Should be able to write again after clear
	rb.Write([]byte("world"))
	data = rb.ReadAll()
	if !bytes.Equal(data, []byte("world")) {
		t.Errorf("expected 'world', got '%s'", string(data))
	}
}
