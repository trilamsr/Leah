import SwiftUI

public struct ErrorState: View {
    private let message: String
    private let retry: (() -> Void)?

    public init(message: String, retry: (() -> Void)? = nil) {
        self.message = message
        self.retry = retry
    }

    public var body: some View {
        VStack(spacing: 12) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 32, weight: .light))
                .foregroundColor(.yellow)
            Text("Something went wrong")
                .font(.system(size: 15, weight: .medium))
                .foregroundColor(.white)
            Text(message)
                .font(.system(size: 12))
                .foregroundColor(.gray)
                .multilineTextAlignment(.center)
            if let retry {
                Button("Retry", action: retry)
                    .padding(.top, 4)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding(32)
    }
}
