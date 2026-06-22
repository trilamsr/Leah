import SwiftUI

public struct WizardView: View {
  @ObservedObject var controller: WizardController

  public init(controller: WizardController) { self.controller = controller }

  public var body: some View {
    VStack {
      switch controller.currentStep {
      case .welcome:     WelcomeStep(onContinue: controller.advance)
      case .apiKey:      APIKeyStep(onContinue: controller.advance)
      case .hotkey:      HotkeyStep(onContinue: controller.advance)
      case .mic:         MicStep(onContinue: controller.advance)
      case .integration: IntegrationStep(onContinue: controller.advance)
      case .ready:       ReadyStep(onContinue: controller.advance)
      case .done:        Color.clear
      }
    }
    .frame(width: 720, height: 520)
    .background(Color(red: 8/255, green: 9/255, blue: 12/255))
  }
}
