package main

import (
	"math/rand"
	"sort"
)

func ShuffledIntSlice(l int, seed int64) []int {
	a := make([]int, l)
	for i := range a {
		a[i] = i
	}
	r := rand.New(rand.NewSource(seed))
	for i := 0; i < l; i++ {
		// use bound [1,l) so 0 can be used if the integer matches
		n := r.Intn(l-1) + 1
		if i == a[n] || a[i] == n {
			n = 0
		}
		a[n], a[i] = a[i], a[n]
	}
	b := make([]int, l)
	copy(b, a)
	rotateSlice(b, r.Intn(l-2)+1)
	dual := &dualSorting{keys: a, values: b}
	sort.Sort(dual)
	return dual.values
}

func rotateSlice[T any](a []T, k int) {
	k %= len(a)
	// Condition below is added.
	if k < 0 {
		k += len(a)
	}
	b := make([]T, len(a))
	copy(b[:k], a[len(a)-k:])
	copy(b[k:], a[:len(a)-k])
	copy(a, b)
}

type dualSorting struct {
	keys, values []int
}

var _ sort.Interface = &dualSorting{}

func (d *dualSorting) Len() int {
	return len(d.keys)
}

func (d *dualSorting) Less(i, j int) bool {
	return d.keys[i] < d.keys[j]
}

func (d *dualSorting) Swap(i, j int) {
	d.keys[i], d.keys[j] = d.keys[j], d.keys[i]
	d.values[i], d.values[j] = d.values[j], d.values[i]
}
