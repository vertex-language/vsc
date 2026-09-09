// Compiled by this compiler, linked against the library above.
import Shapes

func main() -> Int32 {
    // The first requirement, which is the row after the conformance
    // descriptor. Reading the table from its first word instead
    // returns whatever the descriptor's bytes are.
    if aSquare(10).sides() != 4 { return 91 }

    // The second, which is one row further along.
    if aSquare(10).scaled(4) != 40 { return 92 }

    // Three arguments and then the two a witness call adds: the
    // metadata and the table go after what the source wrote, so low
    // and high have to arrive in w0 and w1 and not in w2 and w3.
    if aSquare(6).between(1, 2) != 9 { return 93 }

    // The same three calls on a value too large for the buffer, which
    // is in a box the buffer points at.
    if aFrame(7).sides() != 5 { return 94 }
    if aFrame(7).scaled(3) != 21 { return 95 }
    if aFrame(7).between(10, 20) != 35 { return 96 }

    return aSquare(20).scaled(2) + 2
}
