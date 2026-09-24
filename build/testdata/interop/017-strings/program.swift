// Compiled by this compiler, linked against the library above.
import Text

// A String through a function of this compiler's own: two words in
// and two words out, the same two the library reads.
func longer(_ a: String, _ b: String) -> Int32 {
    if lengthOf(a) > lengthOf(b) { return lengthOf(a) }
    return lengthOf(b)
}

func pick(_ yes: Bool) -> String {
    if yes { return "a longer string than the other one" }
    return "short"
}

// One that owns a heap object, made in one function and used in
// another: the release belongs to whoever ends up holding it, and a
// release too many would be a release of something already gone.
func borrowed() -> String {
    return repeated("xy", 30)
}

func main() -> Int32 {
    if lengthOf(borrowed()) != 60 { return 84 }
    let held = borrowed()
    if lengthOf(held) != 60 { return 85 }
    if lengthOf(echo(held)) != 60 { return 86 }

    if longer("abc", "abcdefghijklmnopqrst") != 20 { return 81 }
    if lengthOf(pick(true)) != 34 { return 82 }
    if lengthOf(pick(false)) != 5 { return 83 }

    // Short enough to be its own bytes.
    if lengthOf("hi") != 2 { return 91 }
    if firstByte("hi") != 104 { return 92 }

    // Fifteen, which is the last length that still fits inline.
    if lengthOf("abcdefghijklmno") != 15 { return 93 }
    if lastByte("abcdefghijklmno") != 111 { return 94 }

    // Sixteen, which does not: the bytes are in the object file and
    // the second word points at them.
    if lengthOf("abcdefghijklmnop") != 16 { return 95 }
    if firstByte("abcdefghijklmnop") != 97 { return 96 }
    if lastByte("abcdefghijklmnop") != 112 { return 97 }

    // Not ASCII, which the initializer is told about and which
    // changes the count: five characters, six bytes.
    if lengthOf("héllo") != 6 { return 98 }

    // Empty.
    if isEmptyString("") != 1 { return 99 }
    if isEmptyString("x") != 0 { return 100 }

    // One handed back and passed on, which is the direction that
    // allocates: what comes back owns a heap object.
    if lengthOf(repeated("ab", 20)) != 40 { return 101 }
    if lengthOf(echo("hello")) != 5 { return 102 }

    // A binding, which is a copy of a String and then a release of
    // it: both go through Swift's bridge-object counting, and a
    // literal's second word is immortal so neither does anything.
    let greeting = "forty and two"
    if lengthOf(greeting) != 13 { return 103 }
    if lengthOf(echo(greeting)) != 13 { return 104 }

    // Its length in characters, which is not its length in bytes:
    // 'é' is two bytes and one character, and the library's own
    // lengthOf counts the bytes.
    if "hello".count != 5 { return 117 }
    if "héllo".count != 5 { return 118 }
    if lengthOf("héllo") != 6 { return 119 }
    if "".count != 0 { return 120 }
    if greeting.count != 13 { return 121 }

    // The operators, which are calls rather than instructions: `+`
    // allocates, and `==` walks two sequences of bytes that may not
    // even be encoded the same way.
    if lengthOf("ab" + "cde") != 5 { return 105 }
    if !("hi" == "hi") { return 106 }
    if "hi" == "ho" { return 107 }
    if !("hi" != "ho") { return 108 }
    if !("abc" < "abd") { return 109 }
    if "abd" < "abc" { return 110 }
    if lengthOf(greeting + " " + echo("more")) != 18 { return 111 }

    // A switch over strings, whose case test is the same call.
    var seen: Int32 = 0
    for _ in 0..<1 {
        switch greeting {
        case "no": seen = 1
        case "forty and two": seen = 2
        default: seen = 3
        }
    }
    if seen != 2 { return 112 }

    // Reassignment, which is a copy of one and a release of what was
    // there before.
    var s = "one"
    if lengthOf(s) != 3 { return 113 }
    s = repeated("ab", 10)
    if lengthOf(s) != 20 { return 114 }
    s = s + "!"
    if lengthOf(s) != 21 { return 115 }
    s += "?"
    if lengthOf(s) != 22 { return 116 }

    return lengthOf("the answer to everything, and more") + 8
}
