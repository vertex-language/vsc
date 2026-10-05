// A generic function this compiler did not declare.
//
// One declared here is monomorphised: the body is lowered once per
// set of type arguments, and the parameter is gone by the time
// anything asks what a value's type is. That needs a body, and an
// imported generic has none -- what the library ships is one function
// that works for every type.
//
// So it is called as it stands, which means telling it what the types
// are. Swift's answer is metadata: the arguments the source wrote,
// then a pointer per type parameter. And the arguments are addresses
// rather than values, because the callee does not know how big T is
// -- swiftc's own code for `tag(x)` on an Int32 puts the address of x
// in x0 and Int32's metadata in x1.
public func tag<T>(_ x: T) -> Int32 { 7 }

public func same<T>(_ x: T) -> T { x }

public func pick<T>(_ a: T, _ b: T, _ first: Bool) -> T { first ? a : b }

public func both<T, U>(_ a: T, _ b: U) -> Int32 { 2 }

// A concrete parameter beside a generic one, which is where the
// argument order and the metadata order are two different orders.
public func after<T>(_ n: Int32, _ x: T) -> T { x }

// A generic result wider than a register, to say that `@out` is about
// the convention and not about the size.
public struct Wide {
    public var a: Int64, b: Int64, c: Int64, d: Int64, e: Int64
    public init(a: Int64) { self.a = a; b = 2; c = 3; d = 4; e = 5 }
}
public func first<T>(_ x: T, _ y: T) -> T { x }
public func widen(_ a: Int64) -> Wide { Wide(a: a) }
public func sumWide(_ w: Wide) -> Int64 { w.a &+ w.b &+ w.c &+ w.d &+ w.e }
