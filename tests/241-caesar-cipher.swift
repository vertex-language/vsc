// Building strings from Unicode scalars: a Caesar cipher.
func shift(_ s: String, by k: Int) -> String {
    var out = ""
    for u in s.unicodeScalars {
        switch u.value {
        case 65...90: out.unicodeScalars.append(UnicodeScalar((Int(u.value) - 65 + k + 26) % 26 + 65)!)
        case 97...122: out.unicodeScalars.append(UnicodeScalar((Int(u.value) - 97 + k + 26) % 26 + 97)!)
        default: out.unicodeScalars.append(u)
        }
    }
    return out
}
let secret = shift("Hello, World!", by: 3)
print(secret, shift(secret, by: -3))
