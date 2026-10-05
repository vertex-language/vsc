// A label lets break and continue name an outer loop.
outer: for i in 1...4 {
    for j in 1...4 {
        if j == 3 { continue outer }
        if i == 3 { break outer }
        print(i, j)
    }
}
print("done")
