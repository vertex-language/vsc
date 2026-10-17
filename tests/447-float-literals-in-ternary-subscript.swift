// Float literals in an array subscripted in one arm of ?: take the type
// the result is wanted as.
for i in 0..<2 {
    for player in [true, false] {
        let skill: Float = player ? 0.97 : [0.985, 0.97][i]
        print(skill)
    }
}
