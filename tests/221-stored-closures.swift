// Closures stored in struct properties, arrays and dictionaries.
struct Button {
    let title: String
    var action: (String) -> String
}
var b = Button(title: "ok") { "pressed \($0)" }
print(b.action(b.title))
b.action = { $0.uppercased() }
print(b.action(b.title))
let ops: [String: (Int, Int) -> Int] = ["+": (+), "-": (-), "max": { max($0, $1) }]
for k in ops.keys.sorted() { print(k, ops[k]!(7, 3)) }
