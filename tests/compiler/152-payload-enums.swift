// A case that carries a value.
//
// The cases share the space the payload goes in, so an enum is the
// largest of them and a byte to say which is there. swiftc's own
// bytes for `enum Shape { case dot; case line(Int32); case box(Int32,
// Int32) }`, read out of a value:
//
//     Shape.dot      = 0 0 0 0 0 0 0 0 2
//     Shape.line(7)  = 7 0 0 0 0 0 0 0 0
//     Shape.box(3,4) = 3 0 0 0 4 0 0 0 1
//
// So the payload sits at offset zero whatever case it belongs to, and
// the tag byte follows the largest of them. The tags are not
// declaration order: the cases that carry something come first, in
// the order they were written, and then the ones that carry nothing.
// That is why line is 0, box is 1 and dot is 2 when dot was written
// first.
//
// Unlike a struct, the fields overlap, so what crosses a call is the
// bytes rather than the fields.
enum Shape {
    case dot
    case line(Int32)
    case box(Int32, Int32)
}

enum Reading {
    case none
    case one(Int64)
}

func area(_ s: Shape) -> Int32 {
    switch s {
    case .dot: return 0
    case .line(let n): return n
    case .box(let w, let h): return w * h
    }
}

func value(_ r: Reading) -> Int64 {
    switch r {
    case .none: return -1
    case .one(let n): return n
    }
}

// An enum beside an ordinary argument, which is where a wrong number
// of registers shows up as a shifted argument list.
func after(_ s: Shape, _ n: Int32) -> Int32 { return n }

func main() -> Int32 {
    if area(.dot) != 0 { return 91 }
    if area(.line(7)) != 7 { return 92 }
    if area(.box(6, 7)) != 42 { return 93 }
    if area(Shape.box(2, 3)) != 6 { return 94 }

    if value(.none) != -1 { return 95 }
    if value(.one(9)) != 9 { return 96 }

    if after(.line(1), 42) != 42 { return 97 }

    // Bound to a name first, then read.
    let s = Shape.box(6, 7)
    if area(s) != 42 { return 98 }

    // A case matched without naming what it carries.
    switch Shape.line(1) {
    case .dot: return 99
    case .box: return 100
    case .line: break
    }

    return area(.box(6, 7))
}
