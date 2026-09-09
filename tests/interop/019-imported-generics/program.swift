// Compiled by this compiler, linked against the library above.
import Poly

func main() -> Int32 {
    // The plainest one: the argument is an address, and the metadata
    // follows it.
    if tag(Int32(3)) != 7 { return 91 }
    if tag(true) != 7 { return 92 }
    if tag(1.5) != 7 { return 93 }

    // A result that is the type parameter, which comes back through
    // storage the caller set aside.
    if same(Int32(42)) != 42 { return 94 }
    if same(Int64(1000000000000)) != 1000000000000 { return 95 }
    if !same(true) { return 96 }
    if same(2.5) != 2.5 { return 97 }

    // Three arguments, one of them concrete, and the metadata after
    // all of them.
    if pick(Int32(1), Int32(2), true) != 1 { return 98 }
    if pick(Int32(1), Int32(2), false) != 2 { return 99 }
    if after(9, Int32(4)) != 4 { return 100 }

    // Two type parameters: two metadata pointers, in the order the
    // signature declares them.
    if both(Int32(1), true) != 2 { return 101 }
    if both(1.5, Int64(2)) != 2 { return 102 }

    // A type argument the library declared rather than the standard
    // library: its metadata is a symbol the library exports, which is
    // the difference between a type declared over there and one
    // declared here.
    if sumWide(first(widen(1), widen(9))) != 15 { return 103 }
    if sumWide(same(widen(6))) != 20 { return 104 }
    if tag(widen(1)) != 7 { return 105 }

    return same(Int32(40)) + tag(Int32(0)) - 5
}
