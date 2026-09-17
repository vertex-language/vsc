// The String operations a package reaches for first, none of which needs
// Foundation: prefixes and suffixes, compared by Character as Swift does,
// appending, decoding bytes, and parsing numbers.
func main() -> Int32 {
    var failures: Int32 = 0

    let s = "hello world"
    if !s.hasPrefix("hell") || !s.hasSuffix("world") || s.hasPrefix("world") { failures += 1 }
    if !s.hasPrefix("") || !s.hasSuffix("") || !s.starts(with: "he") { failures += 1 }
    // By Character: an e with an accent written as two scalars is one
    // Character, which the plain e is not a prefix of.
    let composed = "e\u{301}clair"
    if composed.hasPrefix("e") || !composed.hasPrefix("\u{E9}") || !"caf\u{E9}".hasSuffix("e\u{301}") { failures += 1 }

    var t = "path"
    t.append("/")
    t.append("to")
    t += "/file"
    if t != "path/to/file" { failures += 1 }
    var long = ""
    for _ in 0..<20 {
        long.append("ab")
    }
    if long.count != 40 { failures += 1 }

    let bytes: [UInt8] = [104, 105, 0xE2, 0x82, 0xAC]
    let decoded = String(decoding: bytes, as: UTF8.self)
    if decoded != "hi\u{20AC}" || decoded.count != 3 { failures += 1 }
    let bad: [UInt8] = [0x61, 0xFF, 0x62]
    let repaired = String(decoding: bad, as: UTF8.self)
    if repaired != "a\u{FFFD}b" { failures += 1 }
    if Array("abc".utf8) != [97, 98, 99] { failures += 1 }

    if Int("42") != 42 || Int("-7") != -7 || Int("+3") != 3 { failures += 1 }
    if Int("4 2") != nil || Int("") != nil || Int("99999999999999999999") != nil { failures += 1 }
    if Int32("2147483648") != nil || UInt8("255") != 255 || UInt8("-1") != nil { failures += 1 }
    if Double("2.5") != 2.5 || Double("-1e3") != -1000 || Double("x") != nil { failures += 1 }

    print(t, decoded, repaired, Int("12") ?? 0, Double("0.25") ?? 0)
    print(failures == 0 ? "ok" : "failed \(failures)")
    return failures
}
