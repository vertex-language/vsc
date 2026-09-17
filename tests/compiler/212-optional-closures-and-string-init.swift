// An optional may hold a function -- empty, assigned later, unwrapped
// and called, holding a closure that captures -- and String(x) is what
// interpolating x would write.
func main() -> Int32 {
    var total = 0

    var maybe: ((Int) -> Int)? = nil
    if maybe == nil {
        total += 1
    }
    maybe = { $0 + 1 }
    if let m = maybe {
        total += m(41)
    }

    let base = 10
    let scaled: ((Int) -> Int)? = { $0 * base }
    if let s = scaled {
        total += s(4)
    }

    let empty: ((Int) -> Int)? = nil
    if let e = empty {
        total += e(1000)
    } else {
        total += 100
    }

    let n = 42
    let words = String(n) + " " + String(2.5) + " " + String(true) + " " + String("text")
    if words == "42 2.5 true text" {
        total += 1000
    }
    total += String(n).count

    return Int32(total % 251)
}
