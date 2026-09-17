// §2.3.1 / §2.3.3 Numeric and Regex Literals

// Decimal and Hexadecimal floating-point formats
let hexFloatPosExp = 0x1.0p+10
let hexFloatNegExp = 0x1.8p-4
let hexFloatPlainExp = 0x1p5
let hexFloatMaxFrac = 0x1.fffffffffffffp1023

// Integer base prefixes with extensive separators
let longBinary = 0b1111_0000_1010_0101_1100_0011
let permissions = 0o755
let bigDecimal = 1_000_000_000_000
let longHex = 0xDEAD_BEEF_CAFE_BABE

// Standard Regex Literals
let wordRegex = /[a-zA-Z0-9_]+/
let dateRegex = /\d{4}-\d{2}-\d{2}/
let numberRegex = /[0-9]+/

func testRegexMatching(text: String) -> Bool {
    return text.contains(wordRegex) || text.contains(dateRegex) || text.contains(numberRegex)
}
