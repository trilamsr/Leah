import SwiftUI

public struct WeatherPayload: Equatable {
  public enum Units: String { case imperial, metric }

  public struct Current: Equatable {
    public let temp: Int
    public let condition: String
    public let feelsLike: Int?
    public let windMPH: Int?
  }

  public struct DailyForecast: Equatable {
    public let day: String
    public let condition: String
    public let high: Int
    public let low: Int
  }

  public let location: String
  public let units: Units
  public let current: Current
  public let forecast: [DailyForecast]

  public init(location: String, units: Units, current: Current, forecast: [DailyForecast]) {
    self.location = location; self.units = units
    self.current = current; self.forecast = forecast
  }

  public enum DecodeError: Error { case missing(String) }

  public init(props: [String: AnyCodable]) throws {
    guard let loc = props["location"]?.value as? String else { throw DecodeError.missing("location") }
    let unitsRaw = (props["units"]?.value as? String) ?? "imperial"
    guard let units = Units(rawValue: unitsRaw) else { throw DecodeError.missing("units") }
    guard let currentObj = props["current"]?.value as? [String: AnyCodable] else {
      throw DecodeError.missing("current")
    }
    guard let temp = WeatherPayload.intValue(currentObj["temp"]) else { throw DecodeError.missing("current.temp") }
    guard let cond = currentObj["condition"]?.value as? String else { throw DecodeError.missing("current.condition") }
    let current = Current(
      temp: temp,
      condition: cond,
      feelsLike: WeatherPayload.intValue(currentObj["feels_like"]),
      windMPH: WeatherPayload.intValue(currentObj["wind_mph"])
    )

    guard let forecastArr = props["forecast"]?.value as? [AnyCodable] else {
      throw DecodeError.missing("forecast")
    }
    var days: [DailyForecast] = []
    for (i, item) in forecastArr.enumerated() {
      guard let obj = item.value as? [String: AnyCodable],
            let day = obj["day"]?.value as? String,
            let c = obj["condition"]?.value as? String,
            let hi = WeatherPayload.intValue(obj["high"]),
            let lo = WeatherPayload.intValue(obj["low"]) else {
        throw DecodeError.missing("forecast[\(i)]")
      }
      days.append(DailyForecast(day: day, condition: c, high: hi, low: lo))
    }
    self.init(location: loc, units: units, current: current, forecast: days)
  }

  static func intValue(_ a: AnyCodable?) -> Int? {
    guard let v = a?.value else { return nil }
    if let i = v as? Int { return i }
    if let d = v as? Double { return Int(d.rounded()) }
    return nil
  }
}

public struct WeatherTileView: View {
  public let payload: WeatherPayload

  public init(payload: WeatherPayload) { self.payload = payload }

  public static func iconName(for condition: String) -> String {
    switch condition.lowercased() {
    case "clear", "sun", "sunny":           return "sun.max.fill"
    case "partly_cloudy", "mostly_sunny":   return "cloud.sun.fill"
    case "cloudy", "overcast":              return "cloud.fill"
    case "rain", "showers", "drizzle":      return "cloud.rain.fill"
    case "storm", "thunderstorm":           return "cloud.bolt.fill"
    case "snow", "sleet":                   return "snow"
    case "wind", "windy":                   return "wind"
    case "fog", "mist", "haze":             return "cloud.fog.fill"
    default:                                return "cloud.fill"
    }
  }

  public static func renderedStrings(payload: WeatherPayload) -> [String] {
    var out: [String] = []
    out.append(payload.location)
    out.append("WEATHER")
    out.append(tempLabel(value: payload.current.temp, units: payload.units))
    if let fl = payload.current.feelsLike {
      out.append("feels \(tempLabel(value: fl, units: payload.units))")
    }
    if let w = payload.current.windMPH {
      out.append("wind \(w) mph")
    }
    for d in payload.forecast {
      out.append(d.day)
      out.append(tempLabel(value: d.high, units: payload.units))
      out.append(tempLabel(value: d.low, units: payload.units))
    }
    return out
  }

  static func tempLabel(value: Int, units: WeatherPayload.Units) -> String {
    "\(value)\u{00B0}\(units == .imperial ? "F" : "C")"
  }

  private static let gold     = Color(red: 0xC9/255.0, green: 0xA9/255.0, blue: 0x61/255.0)
  private static let goldMute = Color(red: 0x8A/255.0, green: 0x73/255.0, blue: 0x40/255.0)
  private static let ivory    = Color(red: 0xF2/255.0, green: 0xED/255.0, blue: 0xE0/255.0)
  private static let textDim  = Color(red: 0x8A/255.0, green: 0x84/255.0, blue: 0x78/255.0)

  public var body: some View {
    VStack(alignment: .leading, spacing: 12) {
      HStack(spacing: 8) {
        Text("WEATHER \u{00B7} \(payload.location.uppercased())")
          .font(.caption.weight(.medium))
          .tracking(1.2)
          .foregroundColor(Self.textDim)
        Spacer()
      }

      HStack(alignment: .firstTextBaseline, spacing: 12) {
        Text(Self.tempLabel(value: payload.current.temp, units: payload.units))
          .font(.largeTitle.weight(.regular))
          .foregroundColor(Self.gold)
        Text(payload.current.condition.replacingOccurrences(of: "_", with: " "))
          .font(.callout)
          .foregroundColor(Self.ivory)
      }

      HStack(spacing: 16) {
        ForEach(Array(payload.forecast.enumerated()), id: \.offset) { _, day in
          VStack(spacing: 4) {
            Text(day.day)
              .font(.caption.weight(.medium))
              .foregroundColor(Self.textDim)
            Image(systemName: Self.iconName(for: day.condition))
              .foregroundColor(Self.goldMute)
              .font(.system(size: 16))
            Text(Self.tempLabel(value: day.high, units: payload.units))
              .font(.caption)
              .foregroundColor(Self.ivory)
            Text(Self.tempLabel(value: day.low, units: payload.units))
              .font(.caption)
              .foregroundColor(Self.textDim)
          }
        }
      }
    }
    .padding(12)
  }
}
