// swift-tools-version: 6.0
import PackageDescription

// A TCP package: the sockets are C, the API over them is Swift, and a
// program talks to itself across the loopback interface.
let package = Package(
    name: "TCP",
    products: [
        .library(name: "TCP", targets: ["TCP"]),
        .executable(name: "echo-demo", targets: ["EchoDemo"]),
    ],
    targets: [
        .target(name: "CTcp"),
        .target(name: "TCP", dependencies: ["CTcp"]),
        .executableTarget(name: "EchoDemo", dependencies: ["TCP"]),
    ]
)
