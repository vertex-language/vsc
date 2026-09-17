enum Result2 {
    case success(value: Int, code: Int)
    case failure(String)
}
func describe(_ r: Result2) -> Int {
    switch r {
    case .success(let v, let code): return v + code
    case .failure(let msg): return msg.count
    }
}
