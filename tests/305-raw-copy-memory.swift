// copyMemory(from:byteCount:) between raw pointers.
var src: [UInt8] = [1, 2, 3, 4]
var dst = [UInt8](repeating: 0, count: 6)
dst.withUnsafeMutableBytes { d in
    src.withUnsafeBytes { s in
        d.baseAddress!.advanced(by: 1).copyMemory(from: s.baseAddress!, byteCount: 4)
    }
}
print(dst)
