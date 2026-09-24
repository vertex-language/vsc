// Sorting records by several keys, ascending and descending.
struct Row { let team: String; let points: Int; let name: String }
let rows = [
    Row(team: "red", points: 10, name: "cy"), Row(team: "blue", points: 12, name: "al"),
    Row(team: "red", points: 12, name: "bo"), Row(team: "blue", points: 12, name: "ai"),
    Row(team: "red", points: 10, name: "at"),
]
let sorted = rows.sorted {
    ($0.team, -$0.points, $0.name) < ($1.team, -$1.points, $1.name)
}
for r in sorted { print(r.team, r.points, r.name) }
