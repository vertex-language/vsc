// §6.5 / §4 Concurrency, async/await, and Actors

@globalActor
actor DatabaseActor {
    static let shared = DatabaseActor()
}

actor BankAccount {
    private var balance: Double = 0.0
    nonisolated let accountNumber: String

    init(accountNumber: String) {
        self.accountNumber = accountNumber
    }

    func deposit(amount: Double) {
        balance += amount
    }

    func getBalance() -> Double {
        return balance
    }

    nonisolated func getInfo() -> String {
        return "Account: \(accountNumber)"
    }
}

func transfer(from fromAccount: isolated BankAccount, to toAccount: BankAccount, amount: Double) async {
    fromAccount.deposit(amount: -amount)
    await toAccount.deposit(amount: amount)
}

@DatabaseActor
func queryRecords() async throws -> [String] {
    return ["rec1", "rec2"]
}

func runConcurrently() async {
    async let first = queryRecords()
    async let second = queryRecords()
    _ = try? await [first, second]
}

class LegacyContext {
    nonisolated(unsafe) var unsafePointer: Int = 0
}
