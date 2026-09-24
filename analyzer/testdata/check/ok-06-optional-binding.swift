func f(_ o: Int?) -> Int {
    if let o = o {
        return o
    }
    guard let v = o else { return 0 }
    return v
}
