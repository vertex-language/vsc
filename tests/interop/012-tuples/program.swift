// Compiled by this compiler, linked against the library above.
import Tuples

func main() -> Int32 {
    let t = pairOf(20)
    if t.0 != 20 { return 91 }
    if t.1 != 21 { return 92 }
    if sumTuple(t) != 41 { return 93 }
    if sumTuple((20, 22)) != 42 { return 94 }

    let l = labelled()
    if l.lo != 1 { return 95 }
    if l.hi != 41 { return 96 }

    if after((1, 2), 42) != 42 { return 97 }
    if before(1, (2, 42)) != 42 { return 98 }

    // The same two fields as a struct, which travels packed.
    if vecSum(Vec(x: 20, y: 22)) != 42 { return 99 }

    return sumTuple((20, 22))
}
