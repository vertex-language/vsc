// ?? supplies a default, and chains, and runs its right side only on nil.
func fallback() -> Int {
    print("fallback ran")
    return 0
}
let a: Int? = nil
let b: Int? = 7
print(a ?? 1, b ?? 1, a ?? b ?? 2)
print(b ?? fallback())
print(a ?? fallback())
