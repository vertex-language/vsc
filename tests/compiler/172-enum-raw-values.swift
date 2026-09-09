// `rawValue` on an enum declared with a raw type. The value is not the
// case's tag: the tag is where the case sits among the others, and
// the raw value is what the source wrote. `case bad = 7` is the
// second case carrying a 7, so answering the tag would answer 1.
//
// The raw type was never recorded, so the property did not exist and
// the case values -- which are checked against it -- were checked
// against nothing.
enum Code: Int32 {
    case ok = 1
    case bad = 7
    case worse = 20
}

// Numbered from zero where the declaration says nothing, which is
// Swift's rule.
enum Step: Int32 {
    case first
    case second
    case third
}

// And continuing from the last case that said a number.
enum Port: Int32 {
    case low = 10
    case mid
    case high
}

func main() -> Int32 {
    let codes = Code.ok.rawValue + Code.bad.rawValue + Code.worse.rawValue
    let steps = Step.first.rawValue + Step.second.rawValue + Step.third.rawValue
    let ports = Port.low.rawValue + Port.mid.rawValue + Port.high.rawValue
    return codes + steps + ports
}
