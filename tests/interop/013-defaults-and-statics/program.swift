// Compiled by this compiler, linked against the library above.
import Scaling

func main() -> Int32 {
    if shifted(40) != 42 { return 91 }
    if shifted(40, by: 0) != 40 { return 92 }

    if between(42) != 42 { return 93 }
    if between(-1) != 0 { return 94 }
    if between(-1, low: 7) != 7 { return 95 }
    if between(200, high: 42) != 42 { return 96 }

    // A method's default, with a receiver.
    let s = Scale(factor: 2)
    if s.applied(to: 21) != 42 { return 97 }
    if s.applied(to: 21, times: 2) != 84 { return 98 }

    // Members of the type.
    if Scale.of(3).factor != 3 { return 99 }
    if Scale.identity != 1 { return 100 }

    // Storage of ours, written by code swiftc built.
    var n: Int32 = 40
    bump(&n)
    if n != 41 { return 101 }
    bump(&n, by: 1)
    if n != 42 { return 102 }

    var x: Int32 = 1
    var y: Int32 = 2
    exchange(&x, &y)
    if x != 2 { return 103 }
    if y != 1 { return 104 }

    return shifted(40)
}
