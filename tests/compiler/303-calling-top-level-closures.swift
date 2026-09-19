// A closure held in a top-level let or var is called by name, as one in a
// local is; a var holding one can be given another, and the call made
// after that calls the new one.

let double: (Int) -> Int = { $0 * 2 }
let greet = { (name: String) -> String in "hi " + name }
var counter = 0
var bump: () -> Void = { counter += 1 }
func apply(_ f: (Int) -> Int, _ x: Int) -> Int { return f(x) }
func main() -> Int32 {
    bump()
    bump()
    let before = counter
    bump = { counter += 10 }
    bump()
    print(double(4), greet("bob"), before, counter, apply(double, 21))
    return Int32(double(counter))
}
