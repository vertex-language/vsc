// Identifiable, and looking things up by their id.
struct User: Identifiable, Hashable {
    let id: Int
    var name: String
}
var users = [User(id: 3, name: "c"), User(id: 1, name: "a"), User(id: 2, name: "b")]
let byID = Dictionary(uniqueKeysWithValues: users.map { ($0.id, $0) })
if let i = users.firstIndex(where: { $0.id == 1 }) { users[i].name = "A" }
print(byID[2]!.name, users.sorted { $0.id < $1.id }.map(\.name), Set(users.map(\.id)).count)
