// §9 Parameter Packs in Depth (SE-0393, SE-0398, SE-0399)

struct Bag<each Item> {
    var items: (repeat each Item)

    init(_ items: repeat each Item) {
        self.items = (repeat each items)
    }
}

func transformPacks<each T, each U>(
    values: repeat each T,
    transform: repeat (each T) -> each U
) -> (repeat each U) {
    return (repeat (each transform)(each values))
}

func stringifyPack<each T: CustomStringConvertible>(
    values: repeat each T
) -> [String] {
    return [repeat (each values).description]
}

func zipPacks<each First, each Second>(
    first: repeat each First,
    second: repeat each Second
) -> (repeat (each First, each Second)) {
    return (repeat (each first, each second))
}

func testPackUsage() {
    let bag = Bag(1, "two", 3.0, true)
    _ = bag.items
    let descriptions = stringifyPack(values: 10, 20, 30)
    let zipped = zipPacks(first: 1, "a", second: true, 4.5)
    _ = (descriptions, zipped)
}
