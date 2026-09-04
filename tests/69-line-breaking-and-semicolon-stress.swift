// §5 Semicolon Stress, Empty Statements, and Line-Break Continuations

func testSemicolonAndContinuations(x: Int, y: Int) -> Int {
    var a = x; var b = y; var c = a + b;

    guard a > 0 else { return 0 }; guard b > 0 else { return 1 };

    // Line continuation with binary operator at end of line
    let sum = a +
        b +
        c

    // Line continuation with member access at start of line
    let text = "hello world"
        .uppercased()
        .split(separator: " ")
        .joined(separator: "-")

    // One-line control flow blocks
    if a == b { a += 1; b += 2 } else { a -= 1; b -= 2 };

    while a < 10 { a += 1; if a == 5 { break; }; };

    do { let temp = a; a = b; b = temp; };

    _ = (sum, text, a, b, c)
    return a + b
}
