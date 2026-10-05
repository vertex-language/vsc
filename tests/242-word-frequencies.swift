// A dictionary of counts built from text and reported in a stable order.
let text = "the cat and the hat and the bat sat on the mat"
var freq: [String: Int] = [:]
for w in text.split(separator: " ") { freq[String(w), default: 0] += 1 }
let top = freq.sorted { $0.value != $1.value ? $0.value > $1.value : $0.key < $1.key }
for (w, n) in top.prefix(4) { print(w, n) }
print(freq.count, freq.values.reduce(0, +))
