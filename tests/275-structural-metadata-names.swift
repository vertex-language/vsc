// Two structural types whose spellings differ only in punctuation --
// a tuple (String, Value) and a dictionary [String: Value] -- each get
// their own runtime metadata.
enum Value {
    case n(Int)
    case s(String)
}

let pair: (String, Value) = ("a", .n(1))
let dict: [String: Value] = ["b": .s("x")]
let things: [Any] = [pair, dict]
for t in things {
    if let p = t as? (String, Value) { print("pair", p.0) }
    if let d = t as? [String: Value] { print("dict", d.count) }
}
