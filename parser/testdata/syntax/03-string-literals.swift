// §2.3.2 String Literals

// StaticStringLiteral
let simple = "Hello, Swift!"

// ExtendedStringLiteralDelimiter (raw strings)
let raw = #"No \n escape processing here, and "quotes" are fine."#
let doublyRaw = ##"Can contain a lone # or #"# without ending early"##

// MultilineStringLiteral
let multiline = """
    This spans
    multiple lines.
    "Quotes" are fine unescaped.
    """

// EscapedCharacter examples
let escapes = "Tab:\t Newline:\n Quote:\" Backslash:\\ Unicode:\u{1F600}"

// EscapedNewline (line continuation inside a literal)
let continued = """
    First line \
    continues here without a newline.
    """

// InterpolatedStringLiteral: \ {ExtendedStringLiteralDelimiter} ( Expression )
let name = "World"
let interpolated = "Hello, \(name)! 1 + 1 = \(1 + 1)"

// Interpolation combined with a raw-string delimiter
let rawInterpolated = #"Value: \#(1 + 2), literal backslash: \, literal \(notInterpolated)"#