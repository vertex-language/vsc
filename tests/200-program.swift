// A closing program over the later rungs: an inventory ledger.
enum LedgerError: Error, CustomStringConvertible {
    case unknown(String), insufficient(String, have: Int, want: Int)
    var description: String {
        switch self {
        case .unknown(let s): "unknown item \(s)"
        case let .insufficient(s, have, want): "only \(have) of \(s), wanted \(want)"
        }
    }
}
protocol Priced { var cents: Int { get } }
struct Item: Priced, Hashable, Comparable {
    let name: String
    let cents: Int
    static func < (a: Item, b: Item) -> Bool { (a.cents, a.name) < (b.cents, b.name) }
}
final class Ledger {
    private(set) var stock: [Item: Int] = [:]
    private var log: [String] = []
    var onChange: ((String) -> Void)?
    func add(_ item: Item, _ n: Int = 1) {
        stock[item, default: 0] += n
        note("+\(n) \(item.name)")
    }
    func take(_ name: String, _ n: Int) throws -> Int {
        guard let item = stock.keys.first(where: { $0.name == name }) else { throw LedgerError.unknown(name) }
        let have = stock[item]!
        guard have >= n else { throw LedgerError.insufficient(name, have: have, want: n) }
        stock[item] = have - n
        note("-\(n) \(name)")
        return item.cents * n
    }
    private func note(_ s: String) {
        log.append(s)
        onChange?(s)
    }
    var value: Int { stock.reduce(0) { $0 + $1.key.cents * $1.value } }
    var entries: Int { log.count }
}
let ledger = Ledger()
var changes = 0
ledger.onChange = { _ in changes += 1 }
let apple = Item(name: "apple", cents: 50), pear = Item(name: "pear", cents: 75)
ledger.add(apple, 10)
ledger.add(pear, 4)
ledger.add(Item(name: "fig", cents: 120))
var revenue = 0
for (name, n) in [("apple", 3), ("pear", 5), ("plum", 1), ("fig", 1)] {
    do {
        revenue += try ledger.take(name, n)
    } catch let e as LedgerError {
        print("refused:", e)
    }
}
let remaining = ledger.stock.filter { $0.value > 0 }.keys.sorted()
print(remaining.map { "\($0.name)=\(ledger.stock[$0]!)" }.joined(separator: ", "))
print("revenue \(revenue), value \(ledger.value), entries \(ledger.entries), changes \(changes)")
