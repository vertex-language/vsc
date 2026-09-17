// Values that cross an await and are held in more registers than they
// travel in.
//
// A struct of two Int32s is one word to a caller and two registers to the
// body that reads its fields. Something that crosses a suspension is
// written into the task's frame and read back from it, and reading this
// one back as the one word it arrived in hands a field's worth of code a
// value twice its width -- which links, and is wrong.
//
// A tuple is the same question with a different answer: it travels as its
// elements rather than as a packed word.
struct Pair {
    var a: Int32
    var b: Int32
}

struct Wide {
    var x: Int64
    var y: Int64
    var z: Int64
}

func step(_ n: Int32) async -> Int32 {
    await Task.yield()
    return n
}

// The pair is a parameter, so it arrives in a register and is read after
// the await out of the frame.
func fromParameter(_ p: Pair) async -> Int32 {
    let seen = await step(1)
    return p.a * 10 + p.b + seen
}

// And here it is computed before the await rather than passed in.
func fromLocal(_ n: Int32) async -> Int32 {
    let p = Pair(a: n, b: n + 1)
    let seen = await step(2)
    return p.a * 10 + p.b + seen
}

// A tuple crosses as its elements.
func fromTuple(_ n: Int32) async -> Int32 {
    let t = (n, n * 2)
    let seen = await step(3)
    return t.0 * 10 + t.1 + seen
}

// One too wide for registers is passed by address and read the same way.
func fromWide(_ w: Wide) async -> Int64 {
    let seen = await step(4)
    return w.x + w.y + w.z + Int64(seen)
}

// And one returned that way. An async function with a result too wide for
// registers takes the caller's storage as a parameter and writes through
// it, and hands its continuation nothing -- so after the await the value
// is in that storage and not in any register.
func makeWide(_ n: Int64) async -> Wide {
    let seen = await step(5)
    return Wide(x: n, y: n + 1, z: n + Int64(seen))
}

func main() async -> Int32 {
    var total: Int32 = 0
    total += await fromParameter(Pair(a: 3, b: 4))   // 30 + 4 + 1 = 35
    total += await fromLocal(5)                      // 50 + 6 + 2 = 58
    total += await fromTuple(6)                      // 60 + 12 + 3 = 75
    total += Int32(await fromWide(Wide(x: 1, y: 2, z: 3))) // 6 + 4 = 10
    let w = await makeWide(7)                        // 7, 8, 12
    total += Int32(w.x + w.y + w.z)                  // 27
    return total % 251                               // 205
}
