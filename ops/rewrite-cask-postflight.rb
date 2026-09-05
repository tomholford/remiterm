#!/usr/bin/env ruby
# frozen_string_literal: true

# GoReleaser still emits a deprecated `postflight` block. Homebrew wants
# `postflight_steps`. Rewrite the generated cask in place.

POSTFLIGHT_RE = /^([ \t]*)postflight do\n\1  if OS\.mac\?\n\1    system_command "\/usr\/bin\/xattr", args: \["-dr", "com\.apple\.quarantine", "\#\{staged_path\}\/remiterm"\]\n\1  end\n\1end\n/

POSTFLIGHT_STEPS = <<~'RUBY'
  postflight_steps do
    on_macos do
      run "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "{{staged_path}}/remiterm"]
    end
  end
RUBY

def rewrite(src)
  return src if src.include?("postflight_steps do")

  updated = src.sub(POSTFLIGHT_RE) do
    indent = Regexp.last_match(1)
    POSTFLIGHT_STEPS.rstrip.gsub(/^/, indent) + "\n"
  end
  raise "postflight block not found" if updated == src

  updated
end

if ARGV == ["--test"]
  sample = <<~RUBY
    cask "remiterm" do
      binary "remiterm"

      postflight do
        if OS.mac?
          system_command "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "\#{staged_path}/remiterm"]
        end
      end

      # No zap stanza required
    end
  RUBY
  result = rewrite(sample)
  expected = <<~RUBY
    cask "remiterm" do
      binary "remiterm"

      postflight_steps do
        on_macos do
          run "/usr/bin/xattr", args: ["-dr", "com.apple.quarantine", "{{staged_path}}/remiterm"]
        end
      end

      # No zap stanza required
    end
  RUBY
  raise "rewrite mismatch:\n#{result}" unless result == expected
  puts "ok"
  exit 0
end

path = ARGV.fetch(0)
File.write(path, rewrite(File.read(path)))
