// `each`, `any` and `some` are keywords only where swiftc's parser takes
// them as such: before a name on the same line. Anywhere else they are
// names, and functions called each, any and some are called.

protocol Shape { func area() -> Int }
struct Square: Shape { var side: Int; func area() -> Int { return side * side } }

func each(_ body: () -> Int) -> Int { return body() + body() }
func any(_ s: any Shape) -> Int { return s.area() }
func some(_ n: Int) -> Int { return n + 1 }

func main() -> Int32 {
    var failed: Int32 = 0
    var calls = 0
    let twice = each {
        calls += 1
        return 10
    }
    if twice != 20 || calls != 2 { failed += 1 }
    let shape: any Shape = Square(side: 3)
    if any(shape) != 9 || any(Square(side: 2)) != 4 { failed += 2 }
    if some(1) != 2 { failed += 4 }
    print(twice, any(shape), some(41))
    return failed
}
