// Which accessor a block is decided by its keyword, not by which came
// first. Taking the first accessor with a body emitted the setter's
// as the getter wherever a property wrote `set` before `get` -- and
// that compiled and ran, answering whatever the setter's body left
// behind rather than what the getter says.
struct Reading {
    var raw: Int32

    // set first, which is legal and unusual enough that nothing had
    // written it.
    var doubled: Int32 {
        set { raw = newValue / 2 }
        get { return raw * 2 }
    }

    // get first, the ordinary way round.
    var tripled: Int32 {
        get { return raw * 3 }
        set { raw = newValue / 3 }
    }

    // The implicit getter, which has no keyword at all.
    var negated: Int32 { return -raw }
}

func main() -> Int32 {
    let r = Reading(raw: 7)
    return r.doubled + r.tripled + r.negated
}
