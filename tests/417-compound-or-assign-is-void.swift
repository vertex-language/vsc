// a |= b has no value, as += has none: a one-line closure of it is a
// () -> Void, and one passed to a generic function's body returns Void.
final class C { var w: UInt32 = 1 }
let c = C()
let set: () -> Void = { c.w |= 2 }
set()
print(c.w)
func withLock<R>(_ body: () -> R) -> R { body() }
withLock { c.w |= 8 }
withLock { c.w &= ~1 }
print(c.w)
let r: Void = withLock { c.w ^= 2 }
print(c.w, r)
