// A memberwise initializer may leave a property to its default. The
// default is an expression on the declaration rather than at the
// call, so the call had nothing to lower and every such construction
// was refused -- `S()` where S declares `var n: Int32 = 5` included,
// which is as ordinary as Swift gets.
struct Config {
    var width: Int32 = 10
    var height: Int32 = 20
    var depth: Int32          // no default: always given
}

struct All {
    var a: Int32 = 1
    var b: Int32 = 2
}

// A default that is an expression rather than a literal, since what
// is lowered is the expression the declaration wrote.
struct Derived {
    var base: Int32 = 6 * 7
}

func main() -> Int32 {
    let a = Config(depth: 3)                       // 10, 20, 3
    let b = Config(width: 5, depth: 7)             // 5, 20, 7
    let c = Config(width: 1, height: 2, depth: 3)  // 1, 2, 3
    let d = All()                                  // 1, 2
    let e = All(a: 9)                              // 9, 2

    return a.width + a.depth + b.width + b.height + c.height
         + d.b + e.a + Derived().base
}
