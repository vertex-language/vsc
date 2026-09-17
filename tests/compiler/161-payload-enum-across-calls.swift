// A payload enum crossing a call. Its memory image is words: the
// payload beside the tag. A multi-word one is passed as those words,
// which the ABI arranges out of its leaves -- but a one-word one has
// a single leaf, so it needs a register of its own, and without one
// it was usable inside a function and had no machine type at a
// boundary.
enum Op {
    case add(Int32)
    case scale(Int32)
    case neg
}

func make(_ k: Int32) -> Op {
    return Op.add(k)
}

func apply(_ o: Op, _ n: Int32) -> Int32 {
    switch o {
    case .add(let k): return n + k
    case .scale(let k): return n * k
    case .neg: return -n
    }
}

// In and out of the same function, which is where a representation
// that differed between the two would show.
func roundTrip(_ o: Op) -> Op {
    return o
}

// Two words, which goes the other way: passed as its words.
enum Wide {
    case box(Int32, Int32)
    case empty
}

func area(_ w: Wide) -> Int32 {
    switch w {
    case .box(let a, let b): return a * b
    case .empty: return 0
    }
}

func main() -> Int32 {
    let a = apply(make(5), 10)                  // 15
    let b = apply(roundTrip(Op.neg), 3)         // -3
    let c = apply(roundTrip(make(7)), 1)        // 8
    let d = apply(Op.scale(3), 4)               // 12
    let e = area(Wide.box(2, 5))                // 10
    let f = area(Wide.empty)                    // 0
    return a + b + c + d + e + f
}
