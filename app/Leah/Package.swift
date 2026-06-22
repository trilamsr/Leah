// swift-tools-version:5.9
import PackageDescription

let package = Package(
  name: "Leah",
  platforms: [.macOS(.v14)],
  products: [
    .executable(name: "Leah", targets: ["LeahApp"]),
  ],
  targets: [
    .executableTarget(
      name: "LeahApp",
      dependencies: ["LeahUI"],
      path: "Sources/LeahApp",
      resources: [.process("Leah.entitlements")]
    ),
    .testTarget(
      name: "LeahAppTests",
      dependencies: ["LeahApp"],
      path: "Tests/LeahAppTests"
    ),
    .target(
      name: "LeahUI",
      path: "Sources/LeahUI"
    ),
    .testTarget(
      name: "LeahUITests",
      dependencies: ["LeahUI"],
      path: "Tests/LeahUITests"
    ),
  ]
)
