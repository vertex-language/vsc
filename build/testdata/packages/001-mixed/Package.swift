// swift-tools-version: 6.0
import PackageDescription

// Swift over C and C++: a C target and a C++ target, a Swift library
// that imports the C one, and a program that imports both.
let package = Package(
    name: "Mixed",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "mixed", targets: ["App"]),
        .library(name: "Geometry", type: .static, targets: ["Geometry"]),
    ],
    targets: [
        .target(name: "CMath", cSettings: [.define("SCALE", to: "3")]),
        .target(
            name: "CxxStats",
            cxxSettings: [.headerSearchPath("detail"), .define("STATS_BIAS", to: "1")]
        ),
        .target(name: "Geometry", dependencies: ["CMath"]),
        .executableTarget(
            name: "App",
            dependencies: ["Geometry", .target(name: "CxxStats")],
            linkerSettings: [.linkedLibrary("c++")]
        ),
    ],
    cLanguageStandard: .c11,
    cxxLanguageStandard: .cxx17
)
