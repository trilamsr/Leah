import SwiftUI
import EventKit

// Step 5: ONE integration — Calendar pre-selected (operator decision #14).
// EKEventStore.requestFullAccessToEvents() called only from button action.
public struct IntegrationStep: View {
  let onContinue: () -> Void
  @State private var selected: Integration = .calendar
  @State private var status = ""
  private let store = EKEventStore()

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  enum Integration: String, CaseIterable {
    case calendar = "Calendar"
    case mail     = "Mail"
    case files    = "Files"
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Connect one integration.")
        .font(.system(size: 22, weight: .medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Leah works best with at least one data source.")
        .font(.system(size: 13))
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      ForEach(Integration.allCases, id: \.self) { opt in
        HStack {
          Image(systemName: selected == opt ? "largecircle.fill.circle" : "circle")
            .foregroundColor(selected == opt ? .accentColor : .gray)
          Text(opt.rawValue)
            .font(.system(size: 13))
            .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
        }
        .onTapGesture { selected = opt }
      }
      if !status.isEmpty {
        Text(status).font(.system(size: 12)).foregroundColor(.gray)
      }
      Spacer()
      HStack {
        Spacer()
        Button("Connect & Continue") {
          connectSelected()
        }
      }
    }
    .padding(48)
  }

  private func connectSelected() {
    switch selected {
    case .calendar:
      if #available(macOS 14.0, *) {
        store.requestFullAccessToEvents { ok, _ in
          DispatchQueue.main.async {
            status = ok ? "Calendar connected." : "Calendar access denied."
            if ok { onContinue() }
          }
        }
      } else {
        store.requestAccess(to: .event) { ok, _ in
          DispatchQueue.main.async {
            status = ok ? "Calendar connected." : "Calendar access denied."
            if ok { onContinue() }
          }
        }
      }
    case .mail, .files:
      // Phase 2 integrations — scaffold only; advance immediately.
      onContinue()
    }
  }
}
