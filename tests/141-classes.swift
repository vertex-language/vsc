// A class with stored properties, an initializer and methods.
class Account {
    var owner: String
    var balance: Int
    init(owner: String, balance: Int = 0) {
        self.owner = owner
        self.balance = balance
    }
    func deposit(_ n: Int) { balance += n }
    func describe() -> String { "\(owner): \(balance)" }
}
let a = Account(owner: "ann")
a.deposit(50)
a.deposit(25)
print(a.describe())
