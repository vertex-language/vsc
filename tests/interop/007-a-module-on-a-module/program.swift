// Compiled by this compiler.
//
// It imports one module. The other arrives because Scale's interface
// imports it, which is the thing this case is about: a module read
// because another module needed it, and its types then named the way
// swiftc names them.
//
// The symbol for `widen` is the test. It is declared in Scale and
// takes a type declared in Units, so it mangles with two modules in
// it -- and a symbol that used the calling module for both is one the
// linker cannot find.
import Scale

func main() -> Int32 {
    if origin() != 21 { return 91 }

    // A type from the module nothing here imported.
    let m = Metric(raw: 10)
    if m.raw != 10 { return 92 }
    if m.doubled() != 20 { return 93 }

    if widen(m) != 20 { return 94 }
    if widen(Metric(raw: 21)) != 42 { return 95 }

    let bigger = rescaled(m, by: 4)
    if bigger.raw != 40 { return 96 }

    return widen(Metric(raw: 21))
}
