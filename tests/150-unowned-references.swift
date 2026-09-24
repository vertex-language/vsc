// An unowned reference assumes its object outlives it, and breaks a cycle.
class Customer {
    let name: String
    var card: Card?
    init(_ n: String) { name = n }
    deinit { print("customer gone") }
}
class Card {
    let number: Int
    unowned let owner: Customer
    init(_ n: Int, _ o: Customer) { number = n; owner = o }
    deinit { print("card gone") }
}
var c: Customer? = Customer("bo")
c!.card = Card(1234, c!)
print(c!.card!.owner.name, c!.card!.number)
c = nil
print("end")
