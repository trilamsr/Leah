## Building Leah.app

Requires Xcode 15+ (macOS 14 SDK). Run `make app-build` from the repo root to compile the Swift package in release mode; the output lands at `app/Leah/.build/release/Leah`. Run `make app-test` to execute the unit tests. `make app-run` chains both. Production distribution uses `xcodebuild` with Hardened Runtime signing; see `scripts/sign-and-notarize.sh` (Task 17).
