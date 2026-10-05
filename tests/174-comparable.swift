// Comparable: one < supplies the rest, and makes sorting, min and max work.
struct Version: Comparable, CustomStringConvertible {
    let major, minor: Int
    static func < (a: Version, b: Version) -> Bool { (a.major, a.minor) < (b.major, b.minor) }
    var description: String { "\(major).\(minor)" }
}
enum Priority: Int, Comparable {
    case low = 1, high = 3, medium = 2
    static func < (a: Priority, b: Priority) -> Bool { a.rawValue < b.rawValue }
}
let vs = [Version(major: 1, minor: 10), Version(major: 1, minor: 2), Version(major: 0, minor: 9)]
print(vs.sorted(), vs.max()!, vs.min()!, vs[0] >= vs[1])
print([Priority.high, .low, .medium].sorted(), Priority.low < .high)
