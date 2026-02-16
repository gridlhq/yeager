# typed: false
# frozen_string_literal: true

# Homebrew formula for yeager — remote execution for local AI coding agents.
#
# Install: brew install gridlhq/tap/yeager
# Update:  brew upgrade yeager
#
# SHA256 values and version are updated by the release workflow.

class Yeager < Formula
  desc "Remote execution for local AI coding agents"
  homepage "https://github.com/gridlhq/yeager"
  license "MIT"
  version "0.1.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/gridlhq/yeager/releases/download/v#{version}/yeager_#{version}_darwin_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    else
      url "https://github.com/gridlhq/yeager/releases/download/v#{version}/yeager_#{version}_darwin_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  on_linux do
    if Hardware::CPU.arm?
      url "https://github.com/gridlhq/yeager/releases/download/v#{version}/yeager_#{version}_linux_arm64.tar.gz"
      sha256 "PLACEHOLDER"
    else
      url "https://github.com/gridlhq/yeager/releases/download/v#{version}/yeager_#{version}_linux_amd64.tar.gz"
      sha256 "PLACEHOLDER"
    end
  end

  def install
    bin.install "yg"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/yg --version")
  end
end
