// §2.3.1 Numeric Literals

// IntegerLiteral variants
let binary = 0b1010_1010          // BinaryLiteral with BinaryLiteralCharacter '_'
let octal = 0o17_54                // OctalLiteral
let decimal = 1_000_000            // DecimalLiteral
let hex = 0xFF_EC_DE                // HexadecimalLiteral

// NumericLiteral: '-' IntegerLiteral
let negativeInt = -42

// FloatingPointLiteral: DecimalLiteral [DecimalFraction] [DecimalExponent]
let simpleDouble = 3.14159
let withExponent = 1.25e10
let negativeExponent = 6.022e-23

// FloatingPointLiteral: HexadecimalLiteral [HexadecimalFraction] HexadecimalExponent
let hexFloat = 0x1p10          // 1 * 2^10
let hexFloatFraction = 0x1.8p2 // hexadecimal fraction + exponent

// NumericLiteral: '-' FloatingPointLiteral
let negativeDouble = -0.001