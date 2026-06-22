import SwiftUI

struct SearchBox: View {
    @Binding var query: String

    var body: some View {
        HStack {
            Image(systemName: "magnifyingglass")
                .foregroundColor(.gray)
            TextField("Search settings", text: $query)
                .textFieldStyle(.plain)
                .foregroundColor(.white)
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 6)
        .background(Color(red: 30/255, green: 32/255, blue: 38/255))
        .cornerRadius(6)
    }
}
