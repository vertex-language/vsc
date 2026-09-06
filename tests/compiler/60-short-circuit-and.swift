// && does not evaluate its right side when the left is false.
final class Rec {
    var ran: Int32 = 0
    func yes() -> Bool { ran = ran + 1; return true }
    func no() -> Bool { ran = ran + 10; return false }
}

func main() -> Int32 {
    let r = Rec()
    if r.no() && r.yes() { return 91 }
    return r.ran
}
