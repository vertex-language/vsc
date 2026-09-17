// An existential held in a binding rather than passed straight to a
// call.
//
// What a binding of this type holds is storage: the buffer and the
// table beside it, allocated where the binding begins and written
// through for as long as it lives. That is why writing a different
// conformer into a var has to write the table again -- storing the
// new value over the buffer alone would leave the old type's
// implementation to be reached with the new type's bytes, and the
// answer would be a number that was wrong.
protocol Speaker { func says() -> Int32 }

struct Low: Speaker { func says() -> Int32 { return 1 } }
struct High: Speaker {
    var pitch: Int32
    func says() -> Int32 { return pitch * 10 }
}

func main() -> Int32 {
    let fixed: any Shape2 = Wide(a: 2, b: 3)
    var changing: any Speaker = Low()
    let first = changing.says()

    // The same variable, a different type in it, a different width of
    // value, and a different implementation reached.
    changing = High(pitch: 4)
    let second = changing.says()

    if first != 1 { return 91 }
    if second != 40 { return 92 }
    return fixed.area() + first + second
}

// Declared below what uses them, which is legal and is worth one test
// of its own: the table a value carries is emitted for the module
// rather than for the point of use.
protocol Shape2 { func area() -> Int32 }
struct Wide: Shape2 {
    var a: Int32
    var b: Int32
    func area() -> Int32 { return a * b }
}
