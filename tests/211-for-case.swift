// for case filters a loop by pattern; for-in with where filters by condition.
enum Event { case click(x: Int), key(Character), idle }
let events: [Event] = [.click(x: 3), .key("a"), .idle, .click(x: 9), .key("b")]
for case .click(let x) in events { print("click", x) }
for case let .key(c) in events where c != "a" { print("key", c) }
let maybes: [Int?] = [1, nil, 3]
for case let n? in maybes { print("some", n) }
for i in 1...20 where i % 7 == 0 { print("seven", i) }
