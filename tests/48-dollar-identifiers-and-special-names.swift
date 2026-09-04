// §2.2 Dollar Identifiers and Unicode Names

let anonymousParamSum = [1, 2, 3, 4, 5].reduce(0, { $0 + $1 })
let complexClosure = { $0 * $1 + $2 }(2, 3, 4)

@propertyWrapper
struct Storage<T> {
    var wrappedValue: T
    var projectedValue: String { "storage_projection" }
}

struct Container {
    @Storage var count: Int = 0

    func check() {
        print(count)
        print($count)
    }
}

// Unicode identifiers
let π = 3.141592653589793
let Δx = 0.001
let café = "Bistro"
let résumé = "CV"
let α = 1.0, β = 2.0, γ = 3.0

func computeCircleArea(radius: Double) -> Double {
    return π * radius * radius
}

func testSpecialNames() {
    let c = Container()
    c.check()
    let area = computeCircleArea(radius: 5.0)
    _ = (anonymousParamSum, complexClosure, area, Δx, café, résumé, α, β, γ)
}
