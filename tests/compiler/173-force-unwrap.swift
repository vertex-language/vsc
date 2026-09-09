// `o!` is what the optional holds, or a trap where it holds nothing.
// switch_enum says which; the some arm carries the payload and is
// where everything after the `!` continues, and the none arm does not
// come back -- which is what makes `!` an assertion rather than a
// conversion.
//
// The trap is verified against swiftc separately: both raise SIGTRAP,
// with the same message. A program that takes it cannot also return
// a number to compare.
func force(_ o: Int32?) -> Int32 {
    return o!
}

func firstOr(_ a: Int32?, _ b: Int32?) -> Int32 {
    if a != nil { return a! }
    return b!
}

func chained(_ o: Int32?) -> Int32 {
    return o! * 2 + o!
}

// `nil == o` is `o == nil` the other way round, and only the second
// was accepted: nil is not comparable to anything on its own, so
// asking that of the left operand rejected the order Swift takes.
func present(_ o: Int32?) -> Int32 {
    let a: Int32 = o == nil ? 1 : 0
    let b: Int32 = o != nil ? 10 : 0
    let c: Int32 = nil == o ? 100 : 0
    return a + b + c
}

func main() -> Int32 {
    let five: Int32? = 5
    let seven: Int32? = 7
    return force(five) + firstOr(nil, seven) + chained(five)
         + present(nil) + present(five)
}
