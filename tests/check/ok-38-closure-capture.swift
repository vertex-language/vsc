func makeCounter() -> () -> Int {
    var count = 0
    return {
        count += 1
        return count
    }
}
