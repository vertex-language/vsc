// A closure captures values of any width: a struct of three words, which
// reaches its body in registers, and one of seven words holding a String
// and a class instance, which its body reads where the context keeps it.
// What the context owns is let go with the closure.
var freed = 0

final class Tracker {
    deinit {
        freed += 1
    }
}

struct Big {
    var a: Int
    var b: Int
    var c: Int
    var d: Int
    var name: String
    var t: Tracker
}

struct Three {
    var x: Int
    var y: Int
    var z: Int
}

func makeAdder(_ big: Big) -> () -> Int {
    return { big.a + big.b + big.c + big.d + big.name.count }
}

func run() -> Int {
    let three = Three(x: 1, y: 2, z: 3)
    let offset = 10
    let f = { (n: Int) -> Int in three.x + three.y * three.z + n + offset }
    let g = makeAdder(Big(a: 1, b: 2, c: 3, d: 4, name: "a name long enough for the heap", t: Tracker()))
    return f(7) + g() + g()
}

func main() -> Int32 {
    let r = run()
    return Int32(r % 200 + freed * 100)
}
