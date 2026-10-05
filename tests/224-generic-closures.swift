// Closures whose types are inferred through generic functions.
func transform<T, U>(_ xs: [T], _ f: (T) -> U) -> [U] {
    var out: [U] = []
    for x in xs { out.append(f(x)) }
    return out
}
func pipeline<T>(_ x: T, _ fs: [(T) -> T]) -> T { fs.reduce(x) { $1($0) } }
print(transform([1, 2, 3]) { "n\($0)" })
print(transform(["a", "bb"]) { $0.count > 1 })
print(pipeline(3, [{ $0 + 1 }, { $0 * 10 }, { -$0 }]), pipeline("x", [{ $0 + "y" }, { $0 + $0 }]))
