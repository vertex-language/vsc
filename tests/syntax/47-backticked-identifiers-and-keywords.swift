// §2.2 Backticked Identifiers and Escaped Keywords

let `default` = 10
let `class` = "escaped class"
let `let` = true
let `var` = 3.14

enum `case` {
    case `switch`
    case `enum`
    case `func`
}

struct `struct` {
    var `protocol`: String
    var `self`: Int

    func `init`() -> Int {
        return self.`self`
    }
}

class `class_type` {
    func `guard`(`while`: Int, `for`: Double) -> Int {
        if `while` > 0 {
            return `while`
        }
        return Int(`for`)
    }
}

func testBacktickedIdentifiers() {
    let s = `struct`(protocol: "proto", self: 42)
    _ = s.`init`()
    let c = `case`.`switch`
    let cl = `class_type`()
    _ = cl.`guard`(while: `default`, for: `var`)
    _ = (`class`, `let`, c)
}
