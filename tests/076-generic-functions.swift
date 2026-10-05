// A generic function, instantiated at several types.
func swapped<T, U>(_ pair: (T, U)) -> (U, T) { (pair.1, pair.0) }
func firstOrDefault<T>(_ xs: [T], _ d: T) -> T { xs.isEmpty ? d : xs[0] }
func largest<T: Comparable>(_ xs: [T]) -> T? {
    var best: T? = nil
    for x in xs where best == nil || x > best! { best = x }
    return best
}
print(swapped((1, "a")), firstOrDefault([Int](), 7), firstOrDefault(["x"], "y"))
print(largest([3, 9, 2]) ?? -1, largest(["pear", "apple"]) ?? "", largest([Double]()) as Any)
