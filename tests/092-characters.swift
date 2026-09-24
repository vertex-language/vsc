// Character: its properties and conversions.
for c: Character in ["a", "Z", "7", " ", "é"] {
    print(c, c.isLetter, c.isNumber, c.isUppercase, c.isWhitespace, c.asciiValue as Any)
}
print(Character("a") < Character("b"), String(Character("x")) + "y")
print(Character(UnicodeScalar(65)), Character("5").wholeNumberValue as Any)
