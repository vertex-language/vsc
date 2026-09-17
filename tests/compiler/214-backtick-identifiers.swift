// A keyword written in backticks is an ordinary name: a case, a stored
// property, a static, a function and its label, a parameter, a local a
// closure captures. The backticks are spelling, not part of the name,
// so `default` and default name the same thing where both may be written.
enum Setting {
    case `default`
    case `in`(Int)
    case fast
}

final class Socket {
    var `protocol`: String
    static let `default` = Socket(protocol: "tcp")

    init(protocol p: String) {
        self.`protocol` = p
    }

    func `func`(`inout` value: Int) -> Int {
        return value + `protocol`.count
    }
}

func `return`(_ s: Setting) -> Int {
    switch s {
    case .`default`:
        return 1
    case .`in`(let `var`):
        return `var` * 2
    case .fast:
        return 3
    }
}

func main() -> Int32 {
    let `let` = 5
    let add = { (x: Int) -> Int in x + `let` }
    var total = add(10)
    total += `return`(.default) + `return`(.`in`(21)) + `return`(Setting.fast)
    total += Socket.`default`.func(`inout`: 4)
    let s = Socket(protocol: "udp6")
    s.protocol += "x"
    total += s.`func`(`inout`: 100)
    return Int32(total % 251)
}
