// §4 Complex Operator Sequences, Type Casting, and Ternary Chaining

class Animal {}
class Dog: Animal { func bark() -> String { "woof" } }
class Cat: Animal { func meow() -> String { "meow" } }

func testOperatorPrecedenceAndCasts(a: Int?, b: Int?, flag1: Bool, flag2: Bool, pet: Animal?) -> String {
    // Chained ternary and nil-coalescing
    let val = a ?? (flag1 ? (b ?? (flag2 ? 10 : 20)) : 30)

    // Sequence of casts, force unwraps, and optional chaining
    let sound = (pet as? Dog)?.bark() ?? ((pet as? Cat)?.meow() ?? "unknown")

    // Complex boolean expressions with comparisons and casts
    let isDogOrLarge = (pet is Dog) && (val > 15 || !(val <= 0))

    // Ternary inside array literal and dictionary literal
    let list = [flag1 ? 1 : 2, flag2 ? 3 : 4]
    let dict = [flag1 ? "a" : "b": flag2 ? val : 0]

    // Deep optional chaining with postfix force unwrap
    let nestedOpt: [String: [Int]?]? = ["scores": [100, 200]]
    let firstScore = nestedOpt?["scores"]??[0]

    return "\(val):\(sound):\(isDogOrLarge):\(list.count):\(dict.count):\(firstScore ?? 0)"
}
