struct Box {
    var n = 0
}
extension Box {
    func twice() -> Int {
        return n + n
    }
}
