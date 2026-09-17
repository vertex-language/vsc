// A Swift library that uses Foundation, compiled by swiftc.
//
// The surface is deliberately narrow -- integers in, integers out --
// because that is the boundary this compiler can cross today. What is
// behind it is real: Calendar, Date, TimeZone, UUID, and the ARC and
// generic machinery they are built on, none of which this compiler
// implements. It does not have to. It has to call it correctly.
import Foundation

public func epochYearUTC() -> Int32 {
    var cal = Calendar(identifier: .gregorian)
    cal.timeZone = TimeZone(identifier: "UTC")!
    let parts = cal.dateComponents([.year], from: Date(timeIntervalSince1970: 0))
    return Int32(parts.year ?? 0)
}

public func daysBetweenUTC(_ from: Int32, _ to: Int32) -> Int32 {
    var cal = Calendar(identifier: .gregorian)
    cal.timeZone = TimeZone(identifier: "UTC")!
    let a = Date(timeIntervalSince1970: Double(from) * 86400)
    let b = Date(timeIntervalSince1970: Double(to) * 86400)
    return Int32(cal.dateComponents([.day], from: a, to: b).day ?? -1)
}

public func uuidIsStable() -> Int32 {
    let text = "E621E1F8-C36C-495A-93FC-0C247A3E6E5F"
    guard let id = UUID(uuidString: text) else { return 0 }
    return id.uuidString == text ? 1 : 0
}
