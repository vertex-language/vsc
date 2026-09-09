// A type a protocol names and the conforming type chooses.
//
//     associatedtype Item
//
// Item is not a type. It is a name for whichever type the conformer
// decides it is, and each conformer may decide differently -- which
// is what makes a protocol a description of a family of types rather
// than of one. `Self` is the same idea one level up: the conforming
// type itself, named from inside the protocol.
//
// So `C.Item` in a generic is a dependency and not an answer. Which
// type it is follows from what C turns out to be, and that is decided
// at the call. Monomorphisation is what turns it into an answer: the
// body is lowered once per type argument, and by then C is Ints and
// C.Item is what Ints said Item is.
protocol Container {
    associatedtype Item
    func get() -> Item
    func replaced(_ x: Item) -> Item
}

// A conformer that says outright what Item is.
struct Ints: Container {
    typealias Item = Int32
    func get() -> Int32 { return 7 }
    func replaced(_ x: Int32) -> Int32 { return x * 2 }
}

// A generic whose result type is the associated one, and one whose
// parameter is.
func firstOf<C: Container>(_ c: C) -> C.Item { return c.get() }
func swapped<C: Container>(_ c: C, _ x: C.Item) -> C.Item { return c.replaced(x) }

func main() -> Int32 {
    if firstOf(Ints()) != 7 { return 91 }
    if swapped(Ints(), 20) != 40 { return 92 }
    return firstOf(Ints()) + swapped(Ints(), 15)
}
