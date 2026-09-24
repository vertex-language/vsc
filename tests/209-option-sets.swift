// OptionSet: a set of flags stored in the bits of one integer.
struct Permissions: OptionSet {
    let rawValue: UInt8
    static let read = Permissions(rawValue: 1 << 0)
    static let write = Permissions(rawValue: 1 << 1)
    static let execute = Permissions(rawValue: 1 << 2)
    static let all: Permissions = [.read, .write, .execute]
}
var p: Permissions = [.read]
p.insert(.write)
print(p.rawValue, p.contains(.write), p.contains(.execute), Permissions.all.rawValue)
p.formUnion(.execute)
p.remove(.read)
print(p.rawValue, p == [.write, .execute], p.isSubset(of: .all), p.intersection(.read).isEmpty)
