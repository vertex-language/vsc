// Default argument values, some given and some left out.
func line(_ text: String, width: Int = 10, fill: Character = ".") -> String {
    text + String(repeating: fill, count: max(0, width - text.count))
}
print(line("ab"))
print(line("ab", width: 5))
print(line("ab", fill: "-"))
print(line("abc", width: 6, fill: "*"))
