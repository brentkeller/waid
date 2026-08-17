// Package ids generates the short, human-readable identifiers carried by items in the log.
package ids

import (
	"crypto/rand"
	"fmt"
	"io"
)

// Alphabet is Crockford base32: the digits plus the letters, less i, l, o and u, so an id read
// aloud or copied by hand cannot be confused with another.
const Alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

// Length is the number of characters per item id.
const Length = 4

// MaxAttempts bounds the retry loop, so a saturated or misbehaving exists predicate cannot spin
// forever.
const MaxAttempts = 100

// Generator produces an item id, retrying while exists reports the candidate as taken. A nil exists
// accepts the first candidate. It is the seam tests and the differential harness replace to pin id
// generation to a fixed sequence.
type Generator func(exists func(string) bool) (string, error)

// New generates an item id from the system's cryptographic randomness.
func New(exists func(string) bool) (string, error) {
	return generate(rand.Reader, exists)
}

// Sequence returns a Generator yielding the given ids in order, skipping any the exists predicate
// reports as taken, and failing once the ids run out.
func Sequence(ids ...string) Generator {
	next := 0
	return func(exists func(string) bool) (string, error) {
		for next < len(ids) {
			id := ids[next]
			next++
			if exists == nil || !exists(id) {
				return id, nil
			}
		}
		return "", fmt.Errorf("id sequence exhausted after %d ids", len(ids))
	}
}

func generate(reader io.Reader, exists func(string) bool) (string, error) {
	for attempt := 0; attempt < MaxAttempts; attempt++ {
		id, err := randomID(reader)
		if err != nil {
			return "", err
		}
		if exists == nil || !exists(id) {
			return id, nil
		}
	}
	return "", fmt.Errorf("could not generate a unique id after %d attempts", MaxAttempts)
}

func randomID(reader io.Reader) (string, error) {
	chars := make([]byte, Length)
	for i := range chars {
		index, err := randomIndex(reader, len(Alphabet))
		if err != nil {
			return "", fmt.Errorf("reading randomness for an id: %w", err)
		}
		chars[i] = Alphabet[index]
	}
	return string(chars), nil
}

// randomIndex draws a uniform value below n, which must be in 1..256. Byte values falling in the
// final partial block of n are discarded and redrawn rather than folded with %, which would bias
// the low indices whenever n does not divide 256.
func randomIndex(reader io.Reader, n int) (int, error) {
	limit := 256 - 256%n
	buf := make([]byte, 1)
	for {
		if _, err := io.ReadFull(reader, buf); err != nil {
			return 0, err
		}
		if value := int(buf[0]); value < limit {
			return value % n, nil
		}
	}
}
