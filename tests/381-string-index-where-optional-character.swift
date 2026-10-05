// A String or Substring finds the first or last Character a predicate
// accepts, and an optional Character compares with a literal as a
// Character: `s.first == "#"`.
let line = "  key = value  "
if let i = line.firstIndex(where: { $0 != " " }), let j = line.lastIndex(where: { $0 != " " }) {
    print("[" + String(line[i...j]) + "]")
}
let words = "alpha beta\tgamma"
if let sp = words.firstIndex(where: { $0 == " " || $0 == "\t" }) {
    print(words[..<sp], "|", words[words.index(after: sp)...])
}
let tail = words.dropFirst(6)
print(tail.firstIndex(where: { $0 == "\t" }).map { String(tail[..<$0]) } ?? "none")
print(tail.lastIndex(where: { $0 == "a" }).map { String(tail[$0...]) } ?? "none")
print(words.firstIndex(where: { $0 == "z" }) == nil)

let comment = "# note"
print(comment.first == "#", comment.last == "#", "".first == "#")
let op: Character? = "-"
print(op == "-", op != "+", "-" == op)
