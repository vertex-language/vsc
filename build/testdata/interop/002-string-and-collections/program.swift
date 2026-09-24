import Text

func main() -> Int32 {
    if repeatedLength(4) != 8 { return 91 }
    if wordCount(0) != 3 { return 92 }
    if wordCount(1) != 4 { return 93 }
    if wordCount(9) != -1 { return 94 }
    if sumTo(10) != 55 { return 95 }
    if sortedMiddle() != 5 { return 96 }
    if dictionaryLookup(2) != 20 { return 97 }
    if dictionaryLookup(9) != -1 { return 98 }
    return dictionaryLookup(3) + 30
}
