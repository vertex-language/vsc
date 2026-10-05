func read(_ p: UnsafePointer<Int32>) -> Int32 {
    return p.pointee
}

func write(_ p: UnsafeMutablePointer<Int32>, _ v: Int32) {
    p.pointee = v
}

func nullable(_ p: UnsafeMutablePointer<Int32>?) -> Int32 {
    guard let q = p else { return 0 }
    return q.pointee
}

func convert(_ p: UnsafeMutablePointer<Int32>) -> UnsafeRawPointer {
    return UnsafeRawPointer(OpaquePointer(p))
}

func same(_ a: UnsafeRawPointer, _ b: UnsafeRawPointer) -> Bool {
    return a == b
}

func viaAmpersand() -> Int32 {
    var x: Int32 = 1
    write(&x, 5)
    return read(&x)
}
