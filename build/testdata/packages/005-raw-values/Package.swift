// swift-tools-version: 6.0
import PackageDescription

// Enums with raw values declared in one library -- integers written and
// counted, strings written and implied, a UInt8's -- and a
// program that makes them from raw values and reads them back.
let package = Package(
    name: "RawValues",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "raw-values", targets: ["App"]),
    ],
    targets: [
        .target(name: "Codes"),
        .executableTarget(name: "App", dependencies: ["Codes"]),
    ]
)
