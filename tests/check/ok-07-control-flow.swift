func f(_ n: Int) -> Int {
    var total = 0
    if n > 0 {
        total = n
    } else {
        total = -n
    }
    while total > 10 {
        total = total - 1
    }
    repeat {
        total = total - 1
    } while total > 0
    return total
}
