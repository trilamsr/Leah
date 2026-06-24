import SwiftUI
import MapKit
import CoreLocation

public struct MapsPayload: Codable, Equatable {
  public let lat: Double
  public let lon: Double
  public let label: String

  enum CodingKeys: String, CodingKey { case lat, lon, label }

  public init(lat: Double, lon: Double, label: String) {
    self.lat = lat; self.lon = lon; self.label = label
  }
}

public struct MapsAnnotation: Identifiable, Equatable {
  public let id = UUID()
  public let coordinate: CLLocationCoordinate2D
  public let label: String

  public static func == (lhs: MapsAnnotation, rhs: MapsAnnotation) -> Bool {
    lhs.id == rhs.id
  }
}

public struct MapsTileModel {
  public let payload: MapsPayload
  public let annotations: [MapsAnnotation]
  public let region: MKCoordinateRegion

  public init(payload: MapsPayload) {
    self.payload = payload
    self.annotations = [
      MapsAnnotation(
        coordinate: CLLocationCoordinate2D(latitude: payload.lat, longitude: payload.lon),
        label: payload.label
      ),
    ]
    self.region = MKCoordinateRegion(
      center: CLLocationCoordinate2D(latitude: payload.lat, longitude: payload.lon),
      span: MKCoordinateSpan(latitudeDelta: 0.05, longitudeDelta: 0.05)
    )
  }

  public var goldRegionCount: Int { 1 }
}

public struct MapsTileView: View {
  public let payload: MapsPayload
  private let model: MapsTileModel
  @State private var region: MKCoordinateRegion

  public init(payload: MapsPayload) {
    self.payload = payload
    let m = MapsTileModel(payload: payload)
    self.model = m
    _region = State(initialValue: m.region)
  }

  public var body: some View {
    VStack(alignment: .leading, spacing: 8) {
      eyebrow
      Map(coordinateRegion: $region, annotationItems: model.annotations) { ann in
        MapAnnotation(coordinate: ann.coordinate) {
          ZStack {
            Circle()
              .fill(Color(hex: FlightsTileStyle.goldHex))
              .frame(width: 12, height: 12)
            Circle()
              .stroke(Color(hex: FlightsTileStyle.obsidianHex), lineWidth: 1)
              .frame(width: 12, height: 12)
          }
          .accessibilityLabel(ann.label)
        }
      }
      .frame(height: 180)
      .cornerRadius(8)
    }
    .padding(12)
  }

  private var eyebrow: some View {
    Text("MAPS · \(payload.label.uppercased())")
      .font(.system(.caption, design: .default).weight(.regular))
      .tracking(0.04 * 11)
      .foregroundColor(Color(hex: "#B8B0A0"))
  }
}
