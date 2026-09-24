// switch on String and Character values.
func command(_ s: String) -> String {
    switch s.lowercased() {
    case "start", "go": return "starting"
    case "stop": return "stopping"
    case let other where other.hasPrefix("set "): return "setting " + other.dropFirst(4)
    default: return "unknown"
    }
}
func classify(_ c: Character) -> String {
    switch c {
    case "a", "e", "i", "o", "u": return "vowel"
    case "a"..."z": return "consonant"
    case "0"..."9": return "digit"
    default: return "other"
    }
}
print(command("GO"), command("stop"), command("set speed"), command("?"))
print("hi 5!".map(classify))
