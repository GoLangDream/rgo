package object

import "sync/atomic"

// renderMutationGeneration is a conservative process-wide epoch for
// Ruby-visible mutations that can change a closed-world native serialization
// graph. Native regions use it only as a fast cache guard; a false positive
// merely rebuilds the graph, while a missed mutation would be unsound.
var renderMutationGeneration atomic.Uint64

// CurrentRenderMutationGeneration returns the current Ruby-visible mutation
// epoch. It is deliberately independent from method/constant generations:
// redefining code and changing object data invalidate different caches.
func CurrentRenderMutationGeneration() uint64 {
	return renderMutationGeneration.Load()
}

// BumpRenderMutationGeneration invalidates native serialized-layout caches.
// Callers must invoke it after a successful mutation of an Array, Hash, String,
// or a promoted object ivar that can be observed by a native region.
func BumpRenderMutationGeneration() {
	renderMutationGeneration.Add(1)
}
