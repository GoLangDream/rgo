package object

import "sync/atomic"

var methodGeneration atomic.Uint64

// BumpMethodGeneration invalidates runtime method lookup caches after a class
// or module method table or ancestor chain changes.
func BumpMethodGeneration() {
	methodGeneration.Add(1)
}

// CurrentMethodGeneration returns the generation of the method hierarchy.
func CurrentMethodGeneration() uint64 {
	return methodGeneration.Load()
}
