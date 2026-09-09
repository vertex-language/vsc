// The constraint is checked, not merely written down. A type that
// satisfies its parameter's protocol is accepted; one that does not
// is a diagnostic naming the function, the type and the protocol --
// which is swiftc's message word for word.
//
// This program is the accepting half; the refusing half is in the
// analyzer's own tests, because a program that does not compile has
// no exit status to compare.
protocol Valued { func value() -> Int32 }

struct Yes: Valued { func value() -> Int32 { return 21 } }
struct AlsoYes: Valued { func value() -> Int32 { return 21 } }

func take<T: Valued>(_ x: T) -> Int32 { return x.value() }

func main() -> Int32 {
    return take(Yes()) + take(AlsoYes())
}
