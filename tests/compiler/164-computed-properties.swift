// Computed properties, and the two things that were wrong about them.
//
// A getter's self is @guaranteed the way a method's is: the caller
// keeps the receiver alive across the call and the callee does not
// consume it. Handing over an owned copy instead left, on a class, a
// reference nothing destroyed -- which the verifier rejected as an
// owned value not consumed on all paths.
//
// And a bare computed name inside another member is `self.name`, the
// way a bare stored name is. It resolved and then had no case in
// lowering, so it stopped at the name.
class Box {
    var n: Int32 = 5
    var doubled: Int32 { return n * 2 }
    var quadrupled: Int32 { return doubled * 2 }
    func viaMethod() -> Int32 { return doubled + 1 }
}

struct Vec {
    var x: Int32
    var doubled: Int32 { return x * 2 }
    var quad: Int32 { return doubled * 2 }
    mutating func grow() { x = x + doubled }
}

enum Flag {
    case on
    var one: Int32 { return 1 }
    var two: Int32 { return one * 2 }
}

func take(_ b: Box) -> Int32 { return b.doubled }

func main() -> Int32 {
    let b = Box()
    let viaClass = b.doubled + b.quadrupled + b.viaMethod() + take(b) + Box().doubled

    var v = Vec(x: 3)
    let viaStruct = v.doubled + v.quad
    v.grow()

    return viaClass + viaStruct + v.x + Flag.on.two
}
