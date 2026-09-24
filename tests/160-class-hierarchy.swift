// A small class hierarchy with dynamic dispatch, stored in one array.
class Employee {
    let name: String
    init(_ name: String) { self.name = name }
    func pay() -> Int { 1000 }
    func title() -> String { "employee" }
    final func line() -> String { "\(name), \(title()): \(pay())" }
}
class Manager: Employee {
    var reports: [Employee] = []
    override func pay() -> Int { super.pay() + 200 * reports.count }
    override func title() -> String { "manager" }
}
class Director: Manager {
    override func pay() -> Int { super.pay() * 2 }
    override func title() -> String { "director of " + String(reports.count) }
}
let m = Manager("mia")
m.reports = [Employee("e1"), Employee("e2")]
let d = Director("dan")
d.reports = [m]
let staff: [Employee] = [d, m] + m.reports
for e in staff { print(e.line()) }
print(staff.reduce(0) { $0 + $1.pay() })
