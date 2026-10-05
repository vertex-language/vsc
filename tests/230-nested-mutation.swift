// Mutating deep inside values: a struct in an array in a dictionary in a struct.
struct Item { var qty: Int }
struct Store { var shelves: [String: [Item]] = [:] }
var s = Store()
s.shelves["a"] = [Item(qty: 1), Item(qty: 2)]
s.shelves["a"]![1].qty += 10
s.shelves["b", default: []].append(Item(qty: 7))
s.shelves["a"]?[0].qty = 5
s.shelves["missing"]?[0].qty = 99
let copy = s
s.shelves["b"]![0].qty = 0
print(s.shelves["a"]!.map(\.qty), s.shelves["b"]!.map(\.qty), copy.shelves["b"]!.map(\.qty), s.shelves.count)
