func maybe(_ flag: Bool) -> Int? {
    if flag { return 1 }
    return nil
}
func use() -> Int {
    return maybe(true) ?? 0
}
