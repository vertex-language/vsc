// A value type sharing a class buffer until it is written: copy on write, by hand.
final class Storage {
    var items: [Int]
    init(_ items: [Int]) { self.items = items }
}
struct Bag {
    private var storage = Storage([])
    var items: [Int] { storage.items }
    mutating func add(_ x: Int) {
        if !isKnownUniquelyReferenced(&storage) {
            print("copying")
            storage = Storage(storage.items)
        }
        storage.items.append(x)
    }
}
var a = Bag()
a.add(1)
var b = a
b.add(2)
a.add(3)
a.add(4)
print(a.items, b.items)
