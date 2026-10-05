// dict[key, default: []].append(x) mutates the bucket in place: 200,000
// appends into 500 buckets finish well inside the time limit.
var buckets: [Int: [Int]] = [:]
for i in 0..<200_000 {
    buckets[i % 500, default: []].append(i)
}
var total = 0
for k in 0..<500 { total += buckets[k]!.count }
print(buckets.count, total, buckets[7]!.first!, buckets[7]!.last!)
