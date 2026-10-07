package processor

import "io"

// SetRandReaderForTest allows tests to simulate entropy failure.
func SetRandReaderForTest(r io.Reader) func() {
	prev := randReader
	randReader = r
	return func() {
		randReader = prev
	}
}
