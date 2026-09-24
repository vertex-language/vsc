// swift-tools-version: 6.0
import PackageDescription

// Two libraries that each declare a Cursor -- an enum in one, a struct in
// the other -- and a program that imports both: a case written as `.hidden`
// is the enum's, and a type written through its module is that module's.
let package = Package(
    name: "SameNames",
    platforms: [.macOS(.v13)],
    products: [
        .executable(name: "same-names", targets: ["App"]),
    ],
    targets: [
        .target(name: "Screen"),
        .target(name: "Pointer"),
        .executableTarget(name: "App", dependencies: ["Screen", "Pointer"]),
    ]
)
