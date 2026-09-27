// String equality tells most unequal strings apart by their first
// differing byte, and still finds canonical equivalence where the bytes
// differ past ASCII: composed and decomposed forms, marks after a shared
// prefix, and a string that another continues with a combining mark.
let pairs: [(String, String)] = [
    ("div", "span"), ("abc", "abd"), ("abc", "abcd"), ("", "a"),
    ("caf\u{E9}", "cafe\u{301}"), ("cafe", "caf\u{E9}"), ("e", "e\u{301}"),
    ("a\u{E9}b", "ae\u{301}b"), ("\u{E9}x", "e\u{301}y"), ("\u{1E0B}\u{323}", "\u{1E0D}\u{307}"),
    ("q\u{307}\u{323}", "q\u{323}\u{307}"), ("x\u{E9}", "x\u{E8}"), ("A", "a"),
]
for (a, b) in pairs {
    print(a == b, b == a, a != b)
}
