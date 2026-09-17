func outer(_ n: Int) -> Int {
    func inner(_ m: Int) -> Int {
        return m + 1
    }
    return inner(n)
}
