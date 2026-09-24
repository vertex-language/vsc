// &&, || and !, and that the right side runs only when it has to.
func loud(_ name: String, _ value: Bool) -> Bool {
    print("eval", name)
    return value
}
print(loud("a", false) && loud("b", true))
print(loud("c", true) || loud("d", false))
print(loud("e", true) && loud("f", false))
print(!loud("g", false))
