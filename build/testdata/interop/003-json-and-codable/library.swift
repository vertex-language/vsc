// JSON, through the two paths Swift offers: the untyped
// JSONSerialization and the generic Codable one. Both are behind an
// integer surface, and both bring a good deal of the standard library
// with them.
import Foundation

public func serializationRoundTrip() -> Int32 {
    guard let data = try? JSONSerialization.data(withJSONObject: ["a": 41, "b": 1]),
          let back = try? JSONSerialization.jsonObject(with: data) as? [String: Int]
    else { return -1 }
    return Int32((back["a"] ?? 0) + (back["b"] ?? 0))
}

private struct Reading: Codable {
    var label: String
    var value: Int32
}

public func codableRoundTrip(_ value: Int32) -> Int32 {
    let one = Reading(label: "sample", value: value)
    guard let data = try? JSONEncoder().encode(one),
          let back = try? JSONDecoder().decode(Reading.self, from: data)
    else { return -1 }
    return back.label == "sample" ? back.value : -1
}

public func parseFailureIsReported() -> Int32 {
    let bad = Data("{ not json".utf8)
    return (try? JSONSerialization.jsonObject(with: bad)) == nil ? 1 : 0
}
