// Raw identifiers (SE-0451) and the code points a name may hold.

let `hello world` = 1
let `a+b` = 2
let `123` = 3
let `a.b` = 4
var ` padded ` = 5

func `test that a name may be a sentence`() {}

func `f(x)`() {}

struct `My Struct` {
    var `some property`: Int = 0

    func `do the thing`(with `an argument`: Int) -> Int {
        return `an argument` + `some property`
    }
}

enum `Result Kind` {
    case `not started`
    case `in progress`(percent: Int)
}

protocol `Can Emit` {
    func `emit one`()
}

// A backtick still escapes a keyword, which is its older job.
let `class` = 6
let `if` = 7
func escaped(`in` range: Int, `default` fallback: Int) -> Int { range + fallback }

// Identifiers are not the Unicode letters: an emoji is one, and a
// name may carry combining marks it cannot open with.
let 😀 = 8
let café = 9
let 漢字 = 10
let 🇯🇵flag = 11
let x😀y = 12

func use() -> Int {
    let s = `My Struct`(`some property`: 1)
    let k = `Result Kind`.`in progress`(percent: 50)
    _ = k
    return `hello world` + `a+b` + `123` + `a.b` + ` padded `
        + `class` + `if` + 😀 + café + 漢字 + 🇯🇵flag + x😀y
        + s.`do the thing`(with: 2)
}
