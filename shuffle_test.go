package main

import (
	"math"
	"testing"
)

func TestShuffledIntSlice(t *testing.T) {
	for i := int64(0); i < math.MaxInt16; i++ {
		a := ShuffledIntSlice(16, i)
		for k, v := range a {
			if k == v {
				t.Log(a)
				t.Fatal("Number should not stay in the same position")
			}
		}
	}
}

func FuzzShuffledIntSlice(f *testing.F) {
	f.Fuzz(func(t *testing.T, seed int64) {
		a := ShuffledIntSlice(16, seed)
		for k, v := range a {
			if k == v {
				t.Log(a)
				t.Fatal("Number should not stay in the same position")
			}
		}
	})
}
