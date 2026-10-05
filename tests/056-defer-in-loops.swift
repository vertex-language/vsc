// A defer inside a loop body runs at the end of each pass, even on continue.
for i in 0..<4 {
    defer { print("end of", i) }
    if i == 1 { continue }
    if i == 3 { break }
    print("pass", i)
}
