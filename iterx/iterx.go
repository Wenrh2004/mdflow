// Package iterx is a small set of generic combinators over iter.Seq.
//
// It holds the sequence operations that are not specific to Markdown at all —
// Map, Filter, Reduce and their kin — so the mdflow facade can keep exposing
// only Markdown vocabulary. The name follows the convention of the standard
// library's proposed xiter package: an "x" of extensions over iter.
//
// Go does not allow type parameters on methods, so these live as free
// functions. mdflow.Parser wraps the Event-specialised ones (p.Map, p.Filter)
// for chaining; everything a method chain cannot express is reachable here.
//
// Every combinator is lazy: nothing runs until the sequence is ranged over, and
// breaking out of the range stops the producer at once.
//
// What the standard library already provides is not repeated here: materialise
// a sequence with slices.Collect.
package iterx

import "iter"

// Map returns a sequence with f applied to every element.
func Map[A, B any](seq iter.Seq[A], f func(A) B) iter.Seq[B] {
	return func(yield func(B) bool) {
		for v := range seq {
			if !yield(f(v)) {
				return
			}
		}
	}
}

// FilterMap applies f and keeps only the elements for which it reports ok.
// It is the fused form of Filter followed by Map.
func FilterMap[A, B any](seq iter.Seq[A], f func(A) (B, bool)) iter.Seq[B] {
	return func(yield func(B) bool) {
		for v := range seq {
			if out, ok := f(v); ok {
				if !yield(out) {
					return
				}
			}
		}
	}
}

// Filter returns the elements satisfying pred.
func Filter[A any](seq iter.Seq[A], pred func(A) bool) iter.Seq[A] {
	return func(yield func(A) bool) {
		for v := range seq {
			if pred(v) && !yield(v) {
				return
			}
		}
	}
}

// Reject is Filter with the predicate negated.
func Reject[A any](seq iter.Seq[A], pred func(A) bool) iter.Seq[A] {
	return Filter(seq, func(v A) bool { return !pred(v) })
}

// TakeWhile yields elements until pred first fails, then stops the source.
func TakeWhile[A any](seq iter.Seq[A], pred func(A) bool) iter.Seq[A] {
	return func(yield func(A) bool) {
		for v := range seq {
			if !pred(v) || !yield(v) {
				return
			}
		}
	}
}

// Take yields at most n elements.
func Take[A any](seq iter.Seq[A], n int) iter.Seq[A] {
	return func(yield func(A) bool) {
		if n <= 0 {
			return
		}
		i := 0
		for v := range seq {
			if !yield(v) {
				return
			}
			if i++; i >= n {
				return
			}
		}
	}
}

// Reduce folds seq into a single value.
func Reduce[A, B any](seq iter.Seq[A], init B, f func(B, A) B) B {
	acc := init
	for v := range seq {
		acc = f(acc, v)
	}
	return acc
}

// Each runs f for every element. It is Reduce with no accumulator.
func Each[A any](seq iter.Seq[A], f func(A)) {
	for v := range seq {
		f(v)
	}
}

// Count returns the number of elements.
func Count[A any](seq iter.Seq[A]) int {
	n := 0
	for range seq {
		n++
	}
	return n
}

// Find returns the first element satisfying pred, stopping the source there.
func Find[A any](seq iter.Seq[A], pred func(A) bool) (A, bool) {
	for v := range seq {
		if pred(v) {
			return v, true
		}
	}
	var zero A
	return zero, false
}

// Compose chains transforms left to right: Compose(f, g) applies f then g. It
// is the generic form of what mdflow.Parser.Transform accumulates over its
// Event middlewares.
func Compose[T any](fns ...func(iter.Seq[T]) iter.Seq[T]) func(iter.Seq[T]) iter.Seq[T] {
	switch len(fns) {
	case 0:
		return func(seq iter.Seq[T]) iter.Seq[T] { return seq }
	case 1:
		return fns[0]
	}
	return func(seq iter.Seq[T]) iter.Seq[T] {
		for _, f := range fns {
			seq = f(seq)
		}
		return seq
	}
}
