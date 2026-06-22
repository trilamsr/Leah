import SwiftUI
import LeahIPC

public struct FocusPanelView: View {
  let client: IPCClient
  @State private var input: String = ""
  @State private var response: String = ""

  public init(client: IPCClient) {
    self.client = client
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 12) {
      HStack {
        Text("⬡")
          .font(.system(size: 18, weight: .medium))
          .foregroundColor(Color(red: 201/255, green: 169/255, blue: 97/255))
        TextField("Ask Leah anything…", text: $input)
          .textFieldStyle(.plain)
          .font(.system(size: 18))
          .onSubmit { send() }
      }
      Divider().background(Color.white.opacity(0.20))
      ScrollView {
        Text(response)
          .font(.system(size: 14))
          .foregroundColor(Color(red: 242/255, green: 237/255, blue: 224/255))
          .frame(maxWidth: .infinity, alignment: .leading)
      }
      Spacer()
    }
    .padding(24)
    .background(Color(red: 8/255, green: 9/255, blue: 12/255))
  }

  private func send() {
    let text = input
    input = ""
    response = ""
    Task {
      let turnID = UUID().uuidString
      let askPayload = try JSONEncoder().encode(["text": text])
      let frame = Frame(
        kind: "ask",
        turnId: turnID,
        seq: 0,
        payload: RawJSON(askPayload)
      )
      try await client.send(frame)
      while true {
        let f = try await client.receive()
        if f.kind == "prose.delta",
           let obj = try? JSONDecoder().decode([String: String].self, from: f.payload.data),
           let delta = obj["text"] {
          await MainActor.run { response.append(delta) }
        } else if f.kind == "turn.end" {
          break
        }
      }
    }
  }
}
