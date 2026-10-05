enum Level: Int {
    case low = 1
    case high = 2
}
func use(_ l: Level) -> Int {
    switch l {
    case .low: return 1
    case .high: return 2
    }
}
