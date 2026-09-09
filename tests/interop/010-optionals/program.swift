// Compiled by this compiler, linked against the library above.
//
// Nothing here says `.some`. Swift injects it on the way in, and so
// does this compiler -- both at a binding and at a call.
import Options

func main() -> Int32 {
    if orElse(7, 1) != 7 { return 91 }
    if orElse(nil, 5) != 5 { return 92 }

    let wide: Int64? = 9
    if widthOf(wide, 1) != 9 { return 93 }
    if widthOf(nil, 6) != 6 { return 94 }

    // Nine bytes, which is two registers.
    let pair: Pair? = Pair(a: 3, b: 4)
    if sumOr(pair, 1) != 7 { return 95 }
    if sumOr(nil, 8) != 8 { return 96 }

    if pick(3, 39) != 3 { return 97 }
    if pick(nil, 39) != 39 { return 98 }

    // Read back what swiftc wrote: the tag byte is theirs and the
    // branch on it is this compiler's.
    if let got = maybe(5) {
        if got != 5 { return 99 }
    } else {
        return 100
    }
    if let bad = maybe(-1) { return 101 + bad }

    return orElse(42, 0)
}
