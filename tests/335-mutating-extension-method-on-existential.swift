// A protocol extension's mutating method, called on an existential.
protocol Counter { var count: Int { get set } }
extension Counter {
    mutating func bump(by n: Int) { count += n }
}
struct Clicks: Counter { var count = 0 }
var c: any Counter = Clicks()
c.bump(by: 3)
c.bump(by: 4)
print(c.count)
