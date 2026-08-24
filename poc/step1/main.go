// Command step1 verifies the wildcard candidate generator described in
// docs/lottery-design.md section 4.
//
// A 6-character pattern with k wildcard slots matches exactly 10^k numbers.
// The generator walks the slot space with (start + i*stride) mod 10^k where
// stride shares no prime factor with 10 (not divisible by 2 or 5), which
// visits every candidate exactly once in pseudo-random order while holding
// only three integers of state.
//
// Usage:
//
//	go run ./poc/step1
package main

import (
	"fmt"
	"math/rand"
	"os"
)

const patternLen = 6

func validatePattern(p string) error {
	if len(p) != patternLen {
		return fmt.Errorf("pattern must be %d chars, got %q", patternLen, p)
	}
	for i := 0; i < len(p); i++ {
		if p[i] != '*' && (p[i] < '0' || p[i] > '9') {
			return fmt.Errorf("pattern chars must be digits or '*', got %q", string(p[i]))
		}
	}
	return nil
}

// wildcardSlots returns the indexes of the '*' characters.
func wildcardSlots(p string) []int {
	var slots []int
	for i := 0; i < len(p); i++ {
		if p[i] == '*' {
			slots = append(slots, i)
		}
	}
	return slots
}

// candidate fills the wildcard slots of p with the base-10 digits of
// counter, zero-padded to the slot count.
func candidate(p string, slots []int, counter int) string {
	b := []byte(p)
	for n := len(slots) - 1; n >= 0; n-- {
		b[slots[n]] = byte('0' + counter%10)
		counter /= 10
	}
	return string(b)
}

// matches reports whether number satisfies the pattern.
func matches(pattern, number string) bool {
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '*' && pattern[i] != number[i] {
			return false
		}
	}
	return true
}

func pow10(k int) int {
	n := 1
	for i := 0; i < k; i++ {
		n *= 10
	}
	return n
}

// coprimeStride returns a random stride in [1, space) that is not divisible
// by 2 or 5, the prime factors of 10^k. Such a stride generates the whole
// additive group of integers mod space.
func coprimeStride(space int) int {
	for {
		s := 1 + rand.Intn(space-1)
		if s%2 != 0 && s%5 != 0 {
			return s
		}
	}
}

// walkAll visits every candidate once via the stride walk and returns them
// in visit order.
func walkAll(pattern string, start, stride int) []string {
	slots := wildcardSlots(pattern)
	space := pow10(len(slots))
	out := make([]string, 0, space)
	for i := 0; i < space; i++ {
		out = append(out, candidate(pattern, slots, (start+i*stride)%space))
	}
	return out
}

func demoTinyWalk() {
	fmt.Println("A) tiny walk over a 10-slot space: start=3 stride=7")
	fmt.Print("   visit order: ")
	for i := 0; i < 10; i++ {
		fmt.Printf("%d ", (3+i*7)%10)
	}
	fmt.Println("\n   (every value 0-9 appears exactly once, in scattered order)")
}

func demoRandomOrders(pattern string) {
	slots := wildcardSlots(pattern)
	space := pow10(len(slots))
	fmt.Printf("\nB) pattern %q: k=%d wildcards, candidate space=%d\n", pattern, len(slots), space)
	for run := 1; run <= 2; run++ {
		start := rand.Intn(space)
		stride := coprimeStride(space)
		fmt.Printf("   run %d (start=%d stride=%d), first 8 candidates:", run, start, stride)
		for i := 0; i < 8; i++ {
			fmt.Printf(" %s", candidate(pattern, slots, (start+i*stride)%space))
		}
		fmt.Println()
	}
	fmt.Println("   (two runs produce different orders, so concurrent searchers\n    do not hammer the same numbers first)")
}

// verify exhaustively checks the walk for one pattern against a brute-force
// enumeration of all 1,000,000 numbers.
func verify(pattern string) bool {
	got := walkAll(pattern, rand.Intn(pow10(len(wildcardSlots(pattern)))), coprimeStride(pow10(len(wildcardSlots(pattern)))))

	seen := make(map[string]struct{}, len(got))
	for _, n := range got {
		if _, dup := seen[n]; dup {
			fmt.Printf("   FAIL %q: duplicate candidate %s\n", pattern, n)
			return false
		}
		seen[n] = struct{}{}
		if !matches(pattern, n) {
			fmt.Printf("   FAIL %q: candidate %s violates the pattern\n", pattern, n)
			return false
		}
	}

	want := make(map[string]struct{})
	for v := 0; v < 1_000_000; v++ {
		n := fmt.Sprintf("%06d", v)
		if matches(pattern, n) {
			want[n] = struct{}{}
		}
	}
	if len(seen) != len(want) {
		fmt.Printf("   FAIL %q: generated %d candidates, brute force found %d\n", pattern, len(seen), len(want))
		return false
	}
	for n := range want {
		if _, ok := seen[n]; !ok {
			fmt.Printf("   FAIL %q: missing candidate %s\n", pattern, n)
			return false
		}
	}

	fmt.Printf("   OK %q: %d candidates, all unique, all match, full coverage\n", pattern, len(seen))
	return true
}

func main() {
	demoTinyWalk()

	pattern := "****23"
	demoRandomOrders(pattern)

	fmt.Println("\nC) full verification vs brute force over all 1,000,000 numbers:")
	ok := true
	for _, p := range []string{"****23", "1****5", "123***", "**34**", "******"} {
		if err := validatePattern(p); err != nil {
			fmt.Println("   pattern validation failed:", err)
			os.Exit(1)
		}
		ok = verify(p) && ok
	}

	fmt.Println()
	if !ok {
		fmt.Println("RESULT: FAILED")
		os.Exit(1)
	}
	fmt.Println("RESULT: PASSED - generator is exact, complete and pseudo-random")
}
