func tally(_ words: [String]) -> Int {
    var counts: [String: Int] = [:]
    for w in words {
        counts[w, default: 0] += 1
    }
    var total = 0
    for (word, n) in counts {
        total += word.count * n
    }
    for entry in counts {
        total += entry.value
    }
    for (_, n) in counts {
        total += n
    }
    let seen: Set<String> = ["a"]
    for s in seen {
        total += s.count
    }
    return total + counts["missing", default: 0]
}
