// A local function calling another declared after it, in a do block of
// top-level code and in a function, both capturing a local.
do {
    var scale = 2
    func toScreen(_ x: Int) -> Int { toTablet(x) + 1 }
    func toTablet(_ x: Int) -> Int { x * scale }
    print(toScreen(3))
    scale = 10
    print(toScreen(3))
}
func main() {
    let base = 100
    func first() -> Int { second() + base }
    func second() -> Int { base / 2 }
    print(first())
}
main()
