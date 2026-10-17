// A parameter default that reads a global variable declared above it.
let start = 7
let label = "n"
func f(n: Int = start) -> Int { return n * 2 }
struct P {
    var n: Int
    var tag: String
    init(n: Int = start, tag: String = label) { self.n = n; self.tag = tag }
}
print(f(), f(n: 1))
let p = P()
print(p.tag, p.n)
