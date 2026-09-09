// Comprehensive Swift 6 Kitchen Sink Integration Suite

import Foundation

// Custom Precedence and Operator
precedencegroup ArrowPipelinePrecedence {
    associativity: left
    higherThan: TernaryPrecedence
}

infix operator |>: ArrowPipelinePrecedence

func |> <T, U>(value: T, transform: (T) throws -> U) rethrows -> U {
    return try transform(value)
}

// Ownership and Noncopyable Type
struct UniqueBuffer: ~Copyable {
    private var capacity: Int

    init(capacity: Int) {
        self.capacity = capacity
    }

    deinit {
        // cleanup
    }

    borrowing func readCapacity() -> Int {
        return self.capacity
    }

    consuming func dispose() {
        discard self
    }
}

// Error Enum with Typed Throws
enum PipelineError: Error {
    case invalidInput(String)
    case processingFailed(Int)
}

// Actor Concurrency
actor Aggregator {
    private var accumulator: Double = 0.0

    func add(_ val: Double) {
        accumulator += val
    }

    func total() -> Double {
        accumulator
    }
}

// Protocol with primary associated type and generics
protocol Transforming<Source, Target> {
    associatedtype Source
    associatedtype Target
    func process(_ input: Source) throws(PipelineError) -> Target
}

// Generic Struct conforming with where clause
struct DoubleMapper: Transforming {
    typealias Source = Int
    typealias Target = Double

    func process(_ input: Int) throws(PipelineError) -> Double {
        if input < 0 {
            throw .invalidInput("negative integer")
        }
        return Double(input * 2)
    }
}

// Result Builder
@resultBuilder
struct PipelineBuilder {
    static func buildBlock<each T>(_ components: repeat each T) -> (repeat each T) {
        return (repeat each components)
    }
}

// Function using parameter packs, closures, and trailing closure syntax
func executePack<each Input: CustomStringConvertible>(
    items: repeat each Input
) -> [String] {
    return [repeat (each items).description]
}

// Main integration function exercising language features
func runComprehensivePipeline() async throws {
    let rawString = ##"Config: #"value"#, raw \n path"##
    _ = rawString

    var buffer = UniqueBuffer(capacity: 1024)
    let cap = buffer.readCapacity()
    _ = cap
    buffer.dispose()

    let mapper = DoubleMapper()
    let mapped = try mapper.process(21)

    let piped = try 10 |> { x in Double(x) * 1.5 }

    let actor = Aggregator()
    await actor.add(mapped)
    await actor.add(piped)
    let total = await actor.total()

    let strings = executePack(items: 1, 2.5, "three", true)

    switch total {
    case 0..<50.0:
        print("Normal: \(strings)")
    case 50.0...:
        print("High: \(strings)")
    default:
        break
    }
}
