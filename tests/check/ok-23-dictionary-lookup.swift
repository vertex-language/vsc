func use() -> Int {
    let d = ["a": 1, "b": 2]
    if let v = d["a"] {
        return v
    }
    return 0
}
