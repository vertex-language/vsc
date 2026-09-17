// Compiled by this compiler, linked against the library above.
import Shapes

func main() -> Int32 {
    // A method on a class from another module: self travels in x20.
    // Passing it as an ordinary argument instead made this program
    // answer 113.
    // Made here rather than handed over: a class's allocating
    // initializer takes the type's metadata as its receiver, and that
    // metadata comes from a function the library exports.
    let made = Counter(n: 20)
    if made.plus(1) != 21 { return 90 }

    let c = makeCounter(41)
    if c.plus(1) != 42 { return 91 }
    if c.scaled(by: 2) != 82 { return 92 }

    // A stored property, which is an offset past the header and does
    // not go through the self register at all.
    if c.n != 41 { return 93 }

    // Forty bytes, which come back through the caller's storage.
    let w = makeBig(6)
    if w.a != 6 { return 94 }
    if w.e != 10 { return 95 }

    // And go back the same way.
    let total: Int = sumBig(w)
    if total != 40 { return 96 }

    return Int32(total) + 2
}
