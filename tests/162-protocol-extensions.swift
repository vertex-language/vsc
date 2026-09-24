// A protocol extension supplies default implementations a type may replace.
protocol Describable {
    var id: Int { get }
    func describe() -> String
}
extension Describable {
    func describe() -> String { "item \(id)" }
    func shout() -> String { describe().uppercased() }
}
struct Plain: Describable { let id: Int }
struct Custom: Describable {
    let id: Int
    func describe() -> String { "custom #\(id)" }
}
print(Plain(id: 1).describe(), Custom(id: 2).describe())
print(Plain(id: 3).shout(), Custom(id: 4).shout())
