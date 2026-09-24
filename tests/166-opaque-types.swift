// some P hides a concrete type behind a protocol, for results and parameters.
protocol Container {
    associatedtype Item
    var items: [Item] { get }
}
struct IntBox: Container { var items: [Int] }
func makeBox() -> some Container { IntBox(items: [1, 2, 3]) }
func count(_ c: some Container) -> Int { c.items.count }
func evens(upTo n: Int) -> some Sequence<Int> { stride(from: 0, through: n, by: 2) }
print(count(makeBox()), Array(evens(upTo: 8)))
