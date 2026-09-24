// A variadic parameter arrives as an array, possibly empty.
func sum(_ xs: Int...) -> Int {
    var t = 0
    for x in xs { t += x }
    return t
}
func tag(_ label: String, _ items: String...) -> String {
    label + ":" + items.joined(separator: ",")
}
print(sum(), sum(1), sum(1, 2, 3, 4))
print(tag("empty"), tag("abc", "a", "b", "c"))
