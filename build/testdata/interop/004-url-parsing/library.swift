// URL parsing, which is the networking stack's front door.
//
// No I/O: a test that reaches the network is a test of the network.
// What is exercised is Foundation's own URL and URLComponents, and
// the percent-encoding and query parsing underneath them.
import Foundation

public func portOf(_ which: Int32) -> Int32 {
    // The third is the one that does not parse at all. Foundation is
    // lenient -- "not a url at all" parses, as a relative path -- so
    // the empty string is what actually fails.
    let urls = [
        "https://example.com:8443/a/b?q=1",
        "http://example.com/plain",
        "",
    ]
    let i = Int(which)
    guard i >= 0 && i < urls.count, let u = URL(string: urls[i]) else { return -1 }
    return Int32(u.port ?? 0)
}

public func pathComponentCount(_ which: Int32) -> Int32 {
    let urls = ["https://example.com/a/b/c", "https://example.com/"]
    let i = Int(which)
    guard i >= 0 && i < urls.count, let u = URL(string: urls[i]) else { return -1 }
    // The leading "/" is a component of its own; count the rest.
    return Int32(u.pathComponents.count - 1)
}

public func queryValue(_ key: Int32) -> Int32 {
    let text = "https://example.com/search?page=7&size=35&page=9"
    guard let parts = URLComponents(string: text), let items = parts.queryItems else {
        return -1
    }
    let names = ["page", "size"]
    let i = Int(key)
    guard i >= 0 && i < names.count else { return -1 }
    // The first match, which is what a server would take.
    for item in items where item.name == names[i] {
        return Int32(item.value.flatMap { Int32($0) } ?? -1)
    }
    return -1
}

public func percentEncodes() -> Int32 {
    let raw = "a b&c"
    guard let encoded = raw.addingPercentEncoding(withAllowedCharacters: .alphanumerics) else {
        return 0
    }
    return encoded == "a%20b%26c" ? 1 : 0
}
