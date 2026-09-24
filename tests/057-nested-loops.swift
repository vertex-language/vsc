// Nested loops: a multiplication table.
for i in 1...5 {
    var row = ""
    for j in 1...5 {
        let p = i * j
        row += (p < 10 ? " " : "") + String(p) + " "
    }
    print(row)
}
