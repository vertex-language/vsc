func f(_ values: Int...) -> Int {
    var total = 0
    for v in values { total += v }
    return total
}
func use() -> Int { return f(1, 2, 3) }
