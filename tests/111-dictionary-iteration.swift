// Iterating a dictionary, in an order made deterministic by sorting.
let stock = ["pear": 3, "apple": 5, "fig": 0, "kiwi": 12]
for (name, n) in stock.sorted(by: { $0.key < $1.key }) {
    print(name, n)
}
print(stock.values.reduce(0, +), stock.keys.sorted())
print(stock.filter { $0.value > 2 }.keys.sorted(), stock.mapValues { $0 * 2 }["kiwi"] as Any)
