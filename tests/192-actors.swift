// An actor serializes access to its state across concurrent callers.
actor Bank {
    private var balance = 0
    private(set) var operations = 0
    func deposit(_ n: Int) {
        balance += n
        operations += 1
    }
    func current() -> Int { balance }
}
let bank = Bank()
await withTaskGroup(of: Void.self) { group in
    for i in 1...100 {
        group.addTask { await bank.deposit(i) }
    }
}
print(await bank.current(), await bank.operations)
