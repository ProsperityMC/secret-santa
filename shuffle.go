package main

import "math/rand"

func ShuffledIntSlice(l int, seed int64) []int {
	a := make([]int, l)
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < l; i++ {
		// use bound [1,l) so 0 can be used if the integer matches
		n := r.Intn(l-1) + 1
		if n == i {
			n = 0
		}
		a[i] = n
	}
	return a
}
