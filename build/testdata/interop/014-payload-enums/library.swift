// A case that carries a value, across the boundary.
//
// The cases share the space the payload goes in, so an enum is the
// largest of them and a byte to say which is there. swiftc's own
// bytes for Shape below, read out of a value:
//
//     Shape.dot      = 0 0 0 0 0 0 0 0 2
//     Shape.line(7)  = 7 0 0 0 0 0 0 0 0
//     Shape.box(3,4) = 3 0 0 0 4 0 0 0 1
//
// The tags are not declaration order. The cases that carry something
// come first, in the order they were written, and then the ones that
// carry nothing -- which is why line is 0, box is 1 and dot is 2 when
// dot was written first. Getting that backwards is not a crash: it is
// the wrong arm of a switch, in a program that runs.
public enum Shape {
    case dot
    case line(Int32)
    case box(Int32, Int32)
}

public enum Reading {
    case none
    case one(Int64)
}

public func area(_ s: Shape) -> Int32 {
    switch s {
    case .dot: return 0
    case .line(let n): return n
    case .box(let w, let h): return w * h
    }
}

public func makeLine(_ n: Int32) -> Shape { return .line(n) }
public func makeBox(_ w: Int32, _ h: Int32) -> Shape { return .box(w, h) }
public func reading(_ n: Int64) -> Reading { return n < 0 ? .none : .one(n) }
public func valueOf(_ r: Reading) -> Int64 {
    switch r {
    case .none: return -1
    case .one(let n): return n
    }
}

// An enum beside an ordinary argument, where a wrong number of
// registers shows up as a shifted argument list.
public func after(_ s: Shape, _ n: Int32) -> Int32 { return n }
