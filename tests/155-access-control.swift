// private and fileprivate hide members; private(set) makes a setter private.
struct Wallet {
    private(set) var balance = 0
    private var history: [Int] = []
    mutating func add(_ n: Int) {
        balance += n
        record(n)
    }
    private mutating func record(_ n: Int) { history.append(n) }
    fileprivate var count: Int { history.count }
}
var w = Wallet()
w.add(5)
w.add(7)
print(w.balance, w.count)
