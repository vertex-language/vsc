// A closing program over the later rungs: a bank of accounts replaying a transaction log.
enum TxError: Error, Equatable { case noAccount(String), overdraft(String, Int) }
enum Tx {
    case open(String, Int), deposit(String, Int), withdraw(String, Int), transfer(String, String, Int)
}
protocol Ledger: AnyObject { func record(_ line: String) }
final class Journal: Ledger {
    var lines: [String] = []
    func record(_ line: String) { lines.append(line) }
}
struct Bank {
    private(set) var balances: [String: Int] = [:]
    weak var ledger: Ledger?
    mutating func apply(_ tx: Tx) throws(TxError) {
        switch tx {
        case let .open(name, amount):
            balances[name] = amount
        case let .deposit(name, amount):
            guard balances[name] != nil else { throw .noAccount(name) }
            balances[name]! += amount
        case let .withdraw(name, amount), let .transfer(name, _, amount):
            guard let have = balances[name] else { throw .noAccount(name) }
            guard have >= amount else { throw .overdraft(name, have) }
            if case let .transfer(_, to, _) = tx {
                guard balances[to] != nil else { throw .noAccount(to) }
                balances[to]! += amount
            }
            balances[name] = have - amount
        }
        ledger?.record("\(tx)")
    }
}
let journal = Journal()
var bank = Bank()
bank.ledger = journal
let log: [Tx] = [
    .open("ann", 100), .open("bo", 20), .deposit("bo", 5), .withdraw("ann", 30),
    .transfer("bo", "ann", 50), .transfer("ann", "cy", 1), .deposit("cy", 9), .transfer("ann", "bo", 70),
]
var failures: [TxError] = []
for tx in log {
    do { try bank.apply(tx) } catch { failures.append(error) }
}
for name in bank.balances.keys.sorted() { print(name, bank.balances[name]!) }
print(failures)
print(journal.lines.count, "recorded,", failures.count, "refused, total", bank.balances.values.reduce(0, +))
