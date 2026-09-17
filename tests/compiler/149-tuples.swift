// Several values as one.
//
// A tuple's bytes are a struct's bytes: the elements in order, each
// at the offset its alignment puts it. Swift passes one the way it
// passes the struct with the same elements, so a tuple crossing a
// call needs nothing of its own -- what it needed was for this
// compiler to say that its bytes are that struct's.
//
// `(x)` is not a one-element tuple. Swift has no such thing: it is x
// in parentheses, and the checker types it as x.
struct Pair { var a: Int32; var b: Int32 }

func pairOf(_ n: Int32) -> (Int32, Int32) { return (n, n + 1) }
func sumTuple(_ t: (Int32, Int32)) -> Int32 { return t.0 + t.1 }
func labelled() -> (lo: Int32, hi: Int32) { return (lo: 1, hi: 41) }
func widened(_ t: (Int32, Int64, Int32)) -> Int64 { return t.1 }
func nested(_ t: (Pair, Int32)) -> Int32 { return t.0.a + t.0.b + t.1 }

// A tuple beside an ordinary argument, which is where a wrong layout
// shows up as a shifted argument list rather than a garbled value.
func after(_ t: (Int32, Int32), _ n: Int32) -> Int32 { return n }

func main() -> Int32 {
    let t = pairOf(20)
    if t.0 != 20 { return 91 }
    if t.1 != 21 { return 92 }
    if sumTuple(t) != 41 { return 93 }
    if sumTuple((20, 22)) != 42 { return 94 }

    let l = labelled()
    if l.lo != 1 { return 95 }
    if l.hi != 41 { return 96 }

    if widened((1, 42, 3)) != 42 { return 97 }
    if nested((Pair(a: 20, b: 21), 1)) != 42 { return 98 }
    if after((1, 2), 42) != 42 { return 99 }

    // Parentheses, not a tuple.
    let plain: Int32 = (42)
    return plain
}
