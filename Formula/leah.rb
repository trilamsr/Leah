class Leah < Formula
  desc "Personal chief-of-staff for macOS — CLI, daemon, and HUD"
  homepage "https://github.com/trilamsr/Leah"
  url "https://github.com/trilamsr/Leah/releases/download/v3.3.0/leah-v3.3.0-src.tar.gz"
  version "3.3.0"
  # TODO(release-runbook): fill sha256 after `shasum -a 256 leah-v3.3.0-src.tar.gz`.
  # sha256 ""
  license "MIT"
  head "https://github.com/trilamsr/Leah.git", branch: "main"

  depends_on "go" => :build

  # -s -w -buildid= mirrors the Makefile verify-checksums target so a brew-built
  # binary is byte-reproducible against the release tarball. No -X version inject:
  # version is a hardcoded const in cmd/leah/main.go, so any -X main.* is a no-op.
  def install
    ldflags = "-s -w -buildid="
    %w[leah leah-daemon leah-hud].each do |cmd|
      system "go", "build", "-trimpath", "-ldflags", ldflags, "-o", bin/cmd, "./cmd/#{cmd}"
    end
  end

  test do
    assert_match "3.3.0", shell_output("#{bin}/leah version")
  end
end
