// CaseIterable lists every case, in declaration order.
enum Weekday: String, CaseIterable {
    case mon, tue, wed, thu, fri
}
print(Weekday.allCases.count, Weekday.allCases.map(\.rawValue))
for d in Weekday.allCases where d != .wed { print(d, terminator: " ") }
print()
