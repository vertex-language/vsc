// A closure body can declare a local function and call it, and the
// function sees the closure's parameters and locals.
let make: (Int) -> [Int] = { base in
    var seen = 0
    func step(_ k: Int) -> Int {
        seen += 1
        return base * k + seen
    }
    return [step(1), step(2), step(3)]
}
print(make(10))
func apply(_ f: (String) -> String) -> String { return f("vertex") }
print(apply { s in
    func shout(_ t: String) -> String { return t.uppercased() + "!" }
    return shout(s)
})
