// A tokenizer over a string's characters, producing an enum per token.
enum Token: Equatable { case number(Int), ident(String), symbol(Character) }
func tokenize(_ s: String) -> [Token] {
    var tokens: [Token] = []
    var i = s.startIndex
    while i < s.endIndex {
        let c = s[i]
        if c.isWhitespace { i = s.index(after: i); continue }
        if c.isNumber || c.isLetter {
            var j = i
            while j < s.endIndex && (s[j].isNumber || s[j].isLetter) { j = s.index(after: j) }
            let word = String(s[i..<j])
            tokens.append(Int(word).map(Token.number) ?? .ident(word))
            i = j
        } else {
            tokens.append(.symbol(c))
            i = s.index(after: i)
        }
    }
    return tokens
}
for t in tokenize("let x1 = 42 + (y * 7)") { print(t) }
