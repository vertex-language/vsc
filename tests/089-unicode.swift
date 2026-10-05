// A string's views: characters, Unicode scalars, UTF-8 and UTF-16.
let s = "héllo 👋🏽"
print(s.count, s.unicodeScalars.count, s.utf8.count, s.utf16.count)
print(Array(s.utf8.prefix(4)))
for scalar in "é👋".unicodeScalars { print(scalar.value, terminator: " ") }
print()
let flag: Character = "🇵🇹"
print(flag.unicodeScalars.count, String(flag).count, flag.isASCII, Character("a").isASCII)
