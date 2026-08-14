package object

import "sync/atomic"

var methodGeneration atomic.Uint64
var constantGeneration atomic.Uint64

// BumpMethodGeneration invalidates runtime method lookup caches after a class
// or module method table or ancestor chain changes.
func BumpMethodGeneration() {
	methodGeneration.Add(1)
}

// CurrentMethodGeneration returns the generation of the method hierarchy.
func CurrentMethodGeneration() uint64 {
	return methodGeneration.Load()
}

// BumpConstantGeneration invalidates speculative constant-dependent plans.
// Constant lookup has different mutation points from method lookup, so keep a
// separate epoch instead of flushing every method cache on a constant write.
func BumpConstantGeneration() {
	constantGeneration.Add(1)
}

// CurrentConstantGeneration returns the epoch of the constant hierarchy.
func CurrentConstantGeneration() uint64 {
	return constantGeneration.Load()
}
