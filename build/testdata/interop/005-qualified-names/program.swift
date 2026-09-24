// Compiled by this compiler, linked against the library above.
//
// Every call below is written twice: once with the bare name and once
// through the module. They have to reach the same function -- the
// module's name is not part of the symbol, it is part of finding it.
import Metrics

func main() -> Int32 {
    if mean(4, 8) != 6 { return 91 }
    if Metrics.mean(4, 8) != 6 { return 92 }

    if medianOf(9, 2, 5) != 5 { return 93 }
    if Metrics.medianOf(9, 2, 5) != 5 { return 94 }

    // The library builds a String to answer this one, which this
    // compiler has no String to build.
    if digitsInDescription(-4096) != 4 { return 95 }
    if Metrics.digitsInDescription(1234567) != 7 { return 96 }

    if roundedTenths(46) != 5 { return 97 }
    if Metrics.roundedTenths(44) != 4 { return 98 }

    // A function named through its module, used as a value rather
    // than called.
    let f: (Int32, Int32) -> Int32 = Metrics.mean
    if f(10, 20) != 15 { return 99 }

    // A type named through Swift is the type it already was.
    let total: Swift.Int32 = mean(40, 44) + Swift.Int32(0)
    return total
}
