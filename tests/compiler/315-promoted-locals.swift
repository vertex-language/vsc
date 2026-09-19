// Locals the compiler keeps in registers rather than stack slots: loop
// counters, values set on one arm and read after the join, values carried
// around a loop, and all of them across awaits -- where a value in a
// register has to be written into the task's frame before it suspends and
// read back on the other side, from whichever block reads it next.

func step(_ n: Int) async -> Int {
    await Task.yield()
    return n + 1
}

func scan(_ bytes: [UInt8]) -> Int {
    var j = 0
    while j + 3 < bytes.count {
        if bytes[j] == 13 && bytes[j + 1] == 10 && bytes[j + 2] == 13 && bytes[j + 3] == 10 {
            return j
        }
        j += 1
    }
    return -1
}

func clamp(_ x: Int, _ lo: Int, _ hi: Int) -> Int {
    var y = x
    if y < lo {
        y = lo
    } else if y > hi {
        y = hi
    }
    return y
}

func carried(_ n: Int) async -> Int {
    var a = 0
    var b = 1
    var i = 0
    var odd = false
    while i < n {
        let next = a + b
        a = b
        b = await step(next) - 1
        odd = !odd
        i += 1
    }
    var total = a
    if odd {
        total += await step(100)
    } else {
        total -= await step(10)
    }
    return total
}

func nested(_ rows: Int, _ cols: Int) async -> Int {
    var sum = 0
    var r = 0
    while r < rows {
        var c = 0
        while c < cols {
            if (r + c) % 3 == 0 {
                sum += await step(r * c)
            } else {
                sum += r - c
            }
            c += 1
        }
        r += 1
    }
    return sum
}

func main() async -> Int32 {
    let req: [UInt8] = Array("GET / HTTP/1.1\r\nHost: x\r\n\r\nbody".utf8)
    print(scan(req), scan([1, 2, 3]))
    print(clamp(-5, 0, 10), clamp(5, 0, 10), clamp(50, 0, 10))
    print(await carried(10), await carried(7))
    print(await nested(5, 6))
    var status = scan(req) - 23
    if await carried(3) != 0 {
        status += 1
    }
    return Int32(status)
}
