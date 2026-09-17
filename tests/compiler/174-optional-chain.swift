// `p?.x` is the member where p holds something and nothing where it
// does not, so it is a switch with the read inside one arm -- which
// is what makes a chain a chain: the read only happens where there is
// something to read from. Both arms hand the join an optional,
// because that is what a chain answers whatever the member is.
//
// It answered Invalid before: the `?` wraps whatever it followed and
// p was already optional, so the member was looked for on a doubly
// optional type and found nothing, silently.
struct Reading { var value: Int32 }

func read(_ r: Reading?) -> Int32 {
    return r?.value ?? -1
}

func present(_ r: Reading?) -> Int32 {
    return r?.value != nil ? 1 : 0
}

// A chain feeding a force unwrap, which is two switches over the same
// question. Parenthesised, because `r?.value!` binds the `!` to the
// member in Swift -- and value is not optional, so that is an error
// there. See TODO.md: this compiler takes it as the chain's.
func forced(_ r: Reading?) -> Int32 {
    return (r?.value)!
}

func main() -> Int32 {
    let some: Reading? = Reading(value: 5)
    let none: Reading? = nil
    return read(some) + read(none) + present(some) + present(none) + forced(some)
}
