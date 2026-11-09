# Template: fill url and sha256 per release (goreleaser or `go build` + upload), then publish in a tap.
class Afterthought < Formula
  desc "Finds what you didn't understand in AI coding sessions; weekly learning roadmap"
  homepage "https://github.com/omerrgoren/afterthought"
  license "MIT"
  head "https://github.com/omerrgoren/afterthought.git", branch: "main"

  depends_on "go" => :build

  def install
    system "go", "build", *std_go_args(ldflags: "-s -w"), "./cmd/afterthought"
  end

  def caveats
    <<~EOS
      Run `afterthought install` once to register the global hook.
    EOS
  end

  test do
    assert_match "afterthought", shell_output("#{bin}/afterthought")
  end
end
