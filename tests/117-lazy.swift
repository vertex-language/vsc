// A lazy sequence runs its closures only for the elements asked for.
var calls = 0
let evens = (1...1_000_000).lazy.map { x -> Int in
    calls += 1
    return x * 2
}.filter { $0 % 3 == 0 }
print(Array(evens.prefix(3)), calls)
