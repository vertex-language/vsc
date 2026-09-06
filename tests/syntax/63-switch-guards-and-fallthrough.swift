// §5.3 Switch Guards, Compound Patterns, and Fallthrough

func evaluateRisk(score: Int, flags: [String], category: String) -> String {
    var assessment = "Initial"

    switch score {
    case ..<0:
        assessment = "Invalid"
    case 0...20:
        assessment = "Low"
        if flags.contains("urgent") {
            fallthrough
        }
    case 21...50 where category == "high_risk", 51...75 where flags.count > 2:
        assessment = "Elevated"
        fallthrough
    case 76...100:
        assessment = "Critical"
    default:
        assessment = "Extreme"
    }

    let status = (score, category)
    switch status {
    case (0...50, "internal"):
        assessment += " [Internal]"
    case (51..., let cat) where cat != "guest":
        assessment += " [External \(cat)]"
    default:
        break
    }

    return assessment
}
