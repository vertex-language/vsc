// break leaves the loop; continue skips to the next pass.
var odds: [Int] = []
for i in 0..<100 {
    if i % 2 == 0 { continue }
    if i > 12 { break }
    odds.append(i)
}
print(odds)
