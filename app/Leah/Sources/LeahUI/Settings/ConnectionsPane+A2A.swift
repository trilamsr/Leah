import SwiftUI

// A2APeersSection renders the §5.10 A2A peers block: paired peer list with
// id/name/lastSeen/status, a pair-OTP entry field, per-peer pause/unpair,
// and the §5.4 "Auto-accept peers in same Tailscale net" toggle. The
// section reads + mutates through A2AIPCClient so the UI stays daemon-free
// for previews + tests.
public struct A2APeersSection: View {
    @State private var peers: [A2APeerRow] = []
    @State private var otp: String = ""
    @State private var pendingUnpair: A2APeerRow?
    @State private var pairError: String?
    @State private var autoAccept: Bool
    private let client: A2AIPCClient

    public init(client: A2AIPCClient = PreviewA2AIPCClient()) {
        self.client = client
        _autoAccept = State(initialValue: client.autoAcceptSameNet)
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Toggle("Auto-accept peers in same Tailscale net", isOn: $autoAccept)
                .foregroundColor(.white)
                .onChange(of: autoAccept) { _, on in client.autoAcceptSameNet = on }
            peerList
            Divider()
            pairForm
        }
        .onAppear { peers = client.listPeers() }
        .alert(item: $pendingUnpair) { p in
            Alert(
                title: Text("Unpair \(p.name)?"),
                message: Text("Consent grants for this peer will be revoked."),
                primaryButton: .destructive(Text("Unpair")) { unpair(p) },
                secondaryButton: .cancel()
            )
        }
    }

    private var peerList: some View {
        VStack(alignment: .leading, spacing: 6) {
            if peers.isEmpty {
                Text("No paired peers.")
                    .font(.system(size: 12))
                    .foregroundColor(.gray)
            } else {
                ForEach(peers) { peerRow($0) }
            }
        }
    }

    private func peerRow(_ p: A2APeerRow) -> some View {
        HStack(spacing: 12) {
            Text(p.name).foregroundColor(.white)
            Text(p.id.prefix(8))
                .font(.system(size: 11, design: .monospaced))
                .foregroundColor(.gray)
            Text(Self.dateFormatter.string(from: p.lastSeen))
                .font(.system(size: 11))
                .foregroundColor(.gray)
            Spacer()
            Text(p.status.rawValue)
                .font(.system(size: 11))
                .foregroundColor(p.status == .paused ? .orange : .green)
            Button(p.status == .paused ? "Resume" : "Pause") {
                client.pause(id: p.id)
                peers = client.listPeers()
            }
            Button("Unpair") { pendingUnpair = p }
        }
    }

    private var pairForm: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Pair new peer").font(.system(size: 12, weight: .semibold)).foregroundColor(.white)
            HStack {
                TextField("OTP (NNN-NNN)", text: $otp)
                    .textFieldStyle(.roundedBorder)
                    .frame(maxWidth: 180)
                Button("Pair") { startPair() }
                    .disabled(otp.isEmpty)
            }
            if let err = pairError {
                Text(err).font(.system(size: 11)).foregroundColor(.red)
            }
        }
    }

    private func startPair() {
        if client.pairStart(otp: otp) == nil {
            pairError = "OTP must be 6 digits."
            return
        }
        otp = ""
        pairError = nil
        peers = client.listPeers()
    }

    private func unpair(_ p: A2APeerRow) {
        client.unpair(id: p.id)
        peers = client.listPeers()
    }

    private static let dateFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateStyle = .none
        f.timeStyle = .short
        return f
    }()
}
