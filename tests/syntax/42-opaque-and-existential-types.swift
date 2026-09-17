// §3.3 Opaque and Existential Types

protocol Worker {
    associatedtype Output
    func perform() -> Output
}

struct TextWorker: Worker {
    func perform() -> String { "done" }
}

func makeWorker() -> some Worker {
    return TextWorker()
}

func consumeExistential(worker: any Worker) {
    _ = worker
}

protocol DescribableWorker: Worker where Output: CustomStringConvertible {}

func processComposition(item: any CustomStringConvertible & Sendable) {
    print(item.description)
}

func genericOpaqueArg(collection: some Collection<Int>) -> Int {
    return collection.reduce(0, +)
}

func anyCollectionArg(collection: any Sequence<String>) -> [String] {
    return Array(collection)
}
