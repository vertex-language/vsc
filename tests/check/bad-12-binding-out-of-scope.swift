func f(_ o: Int?) -> Int {
    if let v = o {
        _ = v
    }
    return v
}
