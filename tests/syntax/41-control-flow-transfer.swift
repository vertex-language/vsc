// §5 Statements and Complex Control Flow Transfer

func complexControlFlow(matrix: [[Int]], threshold: Int) -> Int {
    var total = 0

    defer {
        print("Function exiting: total is \(total)")
    }
    defer {
        print("First clean-up step (stack order: second to run)")
    }

    outerLoop: for (rowIndex, row) in matrix.enumerated() {
        innerLoop: for (colIndex, val) in row.enumerated() {
            if val < 0 {
                continue innerLoop
            }
            if val > 1000 {
                break outerLoop
            }
            total += val
            if total > threshold {
                break outerLoop
            }
            _ = (rowIndex, colIndex)
        }
    }

    var counter = 0
    repeat {
        counter += 1
        if counter % 2 == 0 {
            continue
        }
        total += counter
    } while counter < 10

    switch total {
    case 0...10:
        total += 1
        fallthrough
    case 11...20:
        total += 2
    case 21, 22, 23:
        total += 3
    default:
        break
    }

    return total
}
