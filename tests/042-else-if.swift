// A chain of else-if, reaching each arm once.
func grade(_ score: Int) -> String {
    if score >= 90 {
        return "A"
    } else if score >= 80 {
        return "B"
    } else if score >= 70 {
        return "C"
    } else {
        return "F"
    }
}
for s in [95, 85, 75, 10] {
    print(s, grade(s))
}
