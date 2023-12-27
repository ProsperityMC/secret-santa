package main

import (
	"fmt"
	"math"
	"testing"
)

func isInvalid(t *testing.T, a []int) {
	if hasDuplicates(a) {
		t.Log(a)
		t.Fatal("List has duplicates")
	}
	if i, y := hasSamePosition(a); y {
		t.Log(a)
		t.Fatalf("Number %d should not stay in the same position", i)
	}
}

func hasDuplicates(a []int) bool {
	b := make([]int, len(a))
	for i := range a {
		b[a[i]]++
		if b[a[i]] > 1 {
			return true
		}
	}
	return false
}

func hasSamePosition(a []int) (int, bool) {
	for k, v := range a {
		if k == v {
			return k, true
		}
	}
	return 0, false
}

func TestShuffledIntSlice(t *testing.T) {
	for i := int64(0); i < math.MaxInt16; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			a := ShuffledIntSlice(16, i)
			isInvalid(t, a)
		})
	}
}

func FuzzShuffledIntSlice(f *testing.F) {
	f.Fuzz(func(t *testing.T, seed int64) {
		a := ShuffledIntSlice(16, seed)
		isInvalid(t, a)
	})
}
