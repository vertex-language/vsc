// A tiny stack-machine interpreter: enums for instructions, a loop to run them.
enum Op { case push(Int), add, mul, dup, swap, jumpIfZero(Int), dec, print }
func run(_ program: [Op]) {
    var stack: [Int] = []
    var pc = 0
    var steps = 0
    while pc < program.count && steps < 1000 {
        steps += 1
        switch program[pc] {
        case .push(let n): stack.append(n)
        case .add: let b = stack.removeLast(); stack.append(stack.removeLast() + b)
        case .mul: let b = stack.removeLast(); stack.append(stack.removeLast() * b)
        case .dup: stack.append(stack.last!)
        case .swap: stack.swapAt(stack.count - 1, stack.count - 2)
        case .dec: stack[stack.count - 1] -= 1
        case .print: print(stack)
        case .jumpIfZero(let t): if stack.last == 0 { pc = t; continue }
        }
        pc += 1
    }
    print("halted after", steps)
}
// 5! : acc n -> while n != 0 { acc *= n; n -= 1 }
run([.push(1), .push(5), .jumpIfZero(9), .dup, .push(0), .add, .swap, .print, .dec, .print])
run([.push(2), .push(3), .add, .push(7), .mul, .print])
