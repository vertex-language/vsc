// An array literal compared with an optional array, or passed for an
// optional-array parameter, takes the array's element type.
struct Reply { let Ints: [Int32] }
func get(_ ok: Bool) -> Reply? { ok ? Reply(Ints: [0, 0, 32, 32]) : nil }
print(get(true)?.Ints == [0, 0, 32, 32], get(false)?.Ints == [0, 0, 32, 32])
func clear(clearColor: [Float]? = nil) -> String {
    guard let c = clearColor else { return "keep" }
    return c.map { String($0) }.joined(separator: " ")
}
print(clear(), clear(clearColor: [0, 0, 1, 1]))
