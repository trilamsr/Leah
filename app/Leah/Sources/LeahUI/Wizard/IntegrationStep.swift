import SwiftUI
import EventKit

// Step 5: ONE integration — Calendar pre-selected (operator decision #14).
// EKEventStore.requestFullAccessToEvents() called only from button action.
public struct IntegrationStep: View {
  let onContinue: () -> Void
  @State private var selected: Integration = .calendar
  @State private var status = ""
  @State private var calendarState: ConnectionState = .none
  private let store = EKEventStore()

  public init(onContinue: @escaping () -> Void) { self.onContinue = onContinue }

  public enum Integration: String, CaseIterable, Equatable {
    case calendar = "Calendar"
    case mail     = "Mail"
    case files    = "Files"

    public var connectionState: ConnectionState {
      switch self {
      case .calendar: return .none
      case .mail, .files: return .none
      }
    }
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 16) {
      Text("Connect one integration.")
        .font(.title2.weight(.medium))
        .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
      Text("Leah works best with at least one data source.")
        .font(.callout)
        .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
      ForEach(Integration.allCases, id: \.self) { opt in
        HStack(spacing: 10) {
          Image(systemName: selected == opt ? "largecircle.fill.circle" : "circle")
            .foregroundColor(selected == opt ? .accentColor : .gray)
          StatusGlyphView(state: opt == .calendar ? calendarState : .none)
          Text(opt.rawValue)
            .font(.callout)
            .foregroundColor(Color(red: 184/255, green: 176/255, blue: 160/255))
        }
        .onTapGesture { selected = opt }
      }
      if !status.isEmpty {
        Text(status).font(.caption).foregroundColor(.gray)
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
            calendarState = ok ? .connected : .none
            if ok { onContinue() }
          }
        }
      } else {
        store.requestAccess(to: .event) { ok, _ in
          DispatchQueue.main.async {
            status = ok ? "Calendar connected." : "Calendar access denied."
            calendarState = ok ? .connected : .none
            if ok { onContinue() }
          }
        }
      }
    case .mail, .files:
      onContinue()
    }
  }
}
