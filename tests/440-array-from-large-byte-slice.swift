// Array(bytes[a..<b]) of a large byte array is one copy, not an append per
// element: a 2 MB frame's half copied out sixty times stays well inside a
// run's time.
let w = 2048, rows = 1024
var px = [UInt8](repeating: 0, count: w * rows)
for i in 0..<px.count where i % 1000 == 0 { px[i] = UInt8(i / 1000 % 256) }
var sum = 0
for frame in 0..<60 {
    let a = (frame % 8) * w * 64
    let half = Array(px[a..<(a + w * rows / 2)])
    sum += half.count + Int(half[1000 - a % 1000 == 1000 ? 0 : 1000 - a % 1000])
}
print(sum)
