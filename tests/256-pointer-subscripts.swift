// Subscripts on pointers and buffer pointers: reads, writes, compound assignment, iteration.
var a = [1, 2, 3, 4]
a.withUnsafeMutableBufferPointer { buf in
    buf[0] = 10
    buf[1] += 5
    let p = buf.baseAddress!
    p[2] = p[2] * 100
    print(buf.count, Array(buf))
}
a.withUnsafeBufferPointer { buf in
    var sum = 0
    for x in buf { sum += x }
    print(sum, buf[3])
}
