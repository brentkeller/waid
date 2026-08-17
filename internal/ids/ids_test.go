package ids

import (
	"bytes"
	"strings"
	"testing"
)

func TestAlphabetIs32DistinctCharacters(t *testing.T) {
	if len(Alphabet) != 32 {
		t.Fatalf("alphabet length = %d, want 32", len(Alphabet))
	}
	seen := map[byte]bool{}
	for i := 0; i < len(Alphabet); i++ {
		if seen[Alphabet[i]] {
			t.Fatalf("alphabet repeats %q", Alphabet[i])
		}
		seen[Alphabet[i]] = true
	}
}

func TestAlphabetOmitsAmbiguousLetters(t *testing.T) {
	for _, banned := range []string{"i", "l", "o", "u"} {
		if strings.Contains(Alphabet, banned) {
			t.Errorf("alphabet contains %q", banned)
		}
		if strings.Contains(Alphabet, strings.ToUpper(banned)) {
			t.Errorf("alphabet contains %q", strings.ToUpper(banned))
		}
	}
}

func TestGeneratedIdsAreFourCharactersFromTheAlphabet(t *testing.T) {
	for i := 0; i < 200; i++ {
		id, err := New(nil)
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if len(id) != Length {
			t.Fatalf("id %q length = %d, want %d", id, len(id), Length)
		}
		for _, char := range id {
			if !strings.ContainsRune(Alphabet, char) {
				t.Fatalf("id %q contains %q, which is outside the alphabet", id, char)
			}
		}
	}
}

func TestRetriesExactlyAsLongAsExistsReportsTaken(t *testing.T) {
	calls := 0
	var seen []string
	id, err := New(func(candidate string) bool {
		calls++
		seen = append(seen, candidate)
		return calls <= 3
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if calls != 4 {
		t.Fatalf("exists calls = %d, want 4", calls)
	}
	if seen[3] != id {
		t.Fatalf("returned id = %q, want the last candidate %q", id, seen[3])
	}
}

func TestExistsConsultedOnceWhenTheFirstIdIsFree(t *testing.T) {
	calls := 0
	if _, err := New(func(string) bool {
		calls++
		return false
	}); err != nil {
		t.Fatalf("New: %v", err)
	}
	if calls != 1 {
		t.Fatalf("exists calls = %d, want 1", calls)
	}
}

func TestFailsRatherThanLoopingForeverWhenEveryIdIsTaken(t *testing.T) {
	calls := 0
	id, err := New(func(string) bool {
		calls++
		return true
	})
	if err == nil {
		t.Fatalf("New returned %q, want an error", id)
	}
	if !strings.Contains(err.Error(), "unique id") {
		t.Fatalf("error = %q, want it to mention a unique id", err)
	}
	if calls <= 1 {
		t.Fatalf("exists calls = %d, want more than one attempt before giving up", calls)
	}
	if calls >= 1000 {
		t.Fatalf("exists calls = %d, want a bounded number of attempts", calls)
	}
}

// Every byte value maps to an alphabet character exactly as often as every other, which is what
// rejection sampling buys over folding a random byte with %.
func TestEveryByteValueMapsUniformlyOverTheAlphabet(t *testing.T) {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	reader := bytes.NewReader(all)

	counts := map[int]int{}
	for i := 0; i < 256; i++ {
		index, err := randomIndex(reader, len(Alphabet))
		if err != nil {
			t.Fatalf("randomIndex: %v", err)
		}
		counts[index]++
	}

	for index := 0; index < len(Alphabet); index++ {
		if counts[index] != 8 {
			t.Fatalf("index %d drawn %d times, want 8", index, counts[index])
		}
	}
}

// The partial trailing block is discarded rather than folded, so no index is over-represented when
// the range does not divide 256.
func TestRandomIndexRejectsTheTrailingPartialBlock(t *testing.T) {
	reader := bytes.NewReader([]byte{240, 245, 255, 7})
	index, err := randomIndex(reader, 30)
	if err != nil {
		t.Fatalf("randomIndex: %v", err)
	}
	if index != 7 {
		t.Fatalf("index = %d, want 7 — bytes at or above 240 must be rejected, not folded", index)
	}
	if reader.Len() != 0 {
		t.Fatalf("%d bytes left unread, want every rejected byte consumed", reader.Len())
	}
}

func TestGenerateDrawsCharactersInAlphabetOrder(t *testing.T) {
	id, err := generate(bytes.NewReader([]byte{0, 1, 31, 10}), nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if id != "01za" {
		t.Fatalf("id = %q, want %q", id, "01za")
	}
}

func TestSequenceYieldsIdsInOrderAndSkipsTakenOnes(t *testing.T) {
	generator := Sequence("aaaa", "bbbb", "cccc")

	first, err := generator(nil)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first != "aaaa" {
		t.Fatalf("first id = %q, want %q", first, "aaaa")
	}

	second, err := generator(func(candidate string) bool { return candidate == "bbbb" })
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second != "cccc" {
		t.Fatalf("second id = %q, want %q", second, "cccc")
	}

	if _, err := generator(nil); err == nil {
		t.Fatal("expected an error once the sequence is exhausted")
	}
}

func TestNewSatisfiesTheGeneratorSeam(t *testing.T) {
	var generator Generator = New
	id, err := generator(nil)
	if err != nil {
		t.Fatalf("generator: %v", err)
	}
	if len(id) != Length {
		t.Fatalf("id %q length = %d, want %d", id, len(id), Length)
	}
}
