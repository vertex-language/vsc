// A value that may not be there.
//
// `Int32?` is an enum with two cases, and Swift writes it as one:
// SILGen builds a value with `enum $Optional<Int32>, #Optional.some`
// and nothing in the source says so -- `let a: Int32? = 7` is an
// injection Swift performs on the way in, and a compiler that passed
// the seven along unwrapped would hand the callee four bytes of
// payload and a tag byte of whatever followed it.
//
// What the bytes are is swiftc's answer, read out of its own code:
// `takeOpt(7)` writes the payload at offset zero, a tag byte after
// it, and loads the eight bytes into x0. So `Int32?` is `{ Int32,
// UInt8 }` with zero for the case that carries something and one for
// the case that does not, and it is passed the way that struct is
// passed.
//
// Not every optional has a tag byte. One whose payload has a
// representation no value of it uses spends that on the empty case
// and is the same size as what it wraps: a reference's is null, so
// `Box?` is a pointer. `Bool?` is the same idea and is refused --
// swiftc writes the byte 2 for its empty case, which a Bool that is
// one bit cannot hold.
struct Pair { var a: Int32; var b: Int32 }

func take(_ v: Int32?) -> Int32 { return 1 }
func takeWide(_ v: Int64?) -> Int32 { return 2 }
func takePair(_ v: Pair?) -> Int32 { return 3 }

// An optional parameter with an ordinary one after it: the way this
// goes wrong is not a garbled value but a shifted argument list, the
// optional eating a register that belonged to what came next.
func both(_ v: Int32?, _ n: Int32) -> Int32 { return n }

// Reading one back is a switch on which case it holds rather than a
// test of a bit, and the arm that runs gets the payload as a block
// argument -- which is what SILGen writes and where the name binds.
func unwrap(_ v: Int32?) -> Int32 {
    if let x = v { return x }
    return -1
}
func orElse(_ v: Int32?, _ d: Int32) -> Int32 {
    if let x = v { return x } else { return d }
}
func twice(_ a: Int32?, _ b: Int32?) -> Int32 {
    if let x = a {
        if let y = b { return x + y }
        return x
    }
    return 0
}

func main() -> Int32 {
    let a: Int32? = 7
    let none: Int32? = nil
    let wide: Int64? = 9
    let pair: Pair? = Pair(a: 1, b: 2)
    let noPair: Pair? = nil

    if take(a) != 1 { return 91 }
    if take(none) != 1 { return 92 }
    if takeWide(wide) != 2 { return 93 }
    if takePair(pair) != 3 { return 94 }
    if takePair(noPair) != 3 { return 95 }

    // Written at the call rather than bound first, which is the same
    // injection in a different place.
    if take(11) != 1 { return 97 }
    if take(nil) != 1 { return 98 }
    if both(3, 39) != 39 { return 99 }

    if unwrap(7) != 7 { return 81 }
    if unwrap(nil) != -1 { return 82 }
    if orElse(3, 9) != 3 { return 83 }
    if orElse(nil, 9) != 9 { return 84 }
    if twice(1, 2) != 3 { return 85 }
    if twice(1, nil) != 1 { return 86 }
    if twice(nil, 2) != 0 { return 87 }

    return both(nil, 42)
}
