// An implicitly unwrapped optional is what Swift imports a C pointer as
// when the header says nothing about nil: an Optional wherever an Optional
// will do, and the value it holds wherever only that will. And a literal
// in one branch of `? :` is of the other branch's type.
var names: [String] = ["zero", "one"]

func name(_ i: Int) -> String! {
    if i < names.count { return names[i] }
    return nil
}

func length(_ s: String) -> Int { return s.count }

func elapsed(_ from: UInt64, _ to: UInt64) -> UInt64 {
    return to > from ? to - from : 0
}

func main() -> Int32 {
    var failures: Int32 = 0

    // Where the wrapped type is wanted, the value is unwrapped.
    let n: Int = length(name(1))
    if n != 3 { failures += 1 }
    let s: String = name(0)
    if s != "zero" { failures += 1 }
    // A member of it is a member of what it holds.
    if name(1).count != 3 { failures += 1 }

    // Taken as it is, it is an Optional, and nil is still nil.
    let missing = name(5)
    if missing != nil { failures += 1 }
    if let held = name(0) {
        if held != "zero" { failures += 1 }
    } else {
        failures += 1
    }
    if (name(7) ?? "none") != "none" { failures += 1 }

    if elapsed(5, 12) != 7 { failures += 1 }
    if elapsed(12, 5) != 0 { failures += 1 }
    let small: UInt8 = failures == 0 ? 200 : 1
    if small != 200 { failures += 1 }

    print(failures == 0 ? "ok" : "failed")
    return failures
}
