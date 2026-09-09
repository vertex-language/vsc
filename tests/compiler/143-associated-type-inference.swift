// The conformer usually does not say what the associated type is --
// it writes the implementation, and that says it.
//
//     struct Ints: Container {
//         func get() -> Int32 { ... }    // so Item is Int32
//     }
//
// swiftc reads it that way and most Swift code relies on it, so this
// does too: the implementation's signature is matched against the
// requirement's, and where the requirement says the associated type
// the implementation says what it is. A property requirement says it
// just as plainly as a method does.
//
// Two conformers choosing differently is the whole point, and is what
// this checks: the same generic, called at both, gives two answers
// because two different bodies were lowered.
protocol Container {
    associatedtype Item
    func get() -> Item
    var head: Item { get }
}

struct Ints: Container {
    var head: Int32
    func get() -> Int32 { return head * 3 }
}
struct Flags: Container {
    var head: Bool
    func get() -> Bool { return !head }
}

func firstOf<C: Container>(_ c: C) -> C.Item { return c.get() }
func headOf<C: Container>(_ c: C) -> C.Item { return c.head }

func main() -> Int32 {
    if firstOf(Ints(head: 4)) != 12 { return 91 }
    if headOf(Ints(head: 9)) != 9 { return 92 }
    if firstOf(Flags(head: true)) { return 93 }
    if !headOf(Flags(head: true)) { return 94 }
    return firstOf(Ints(head: 14))
}
