# frozen_string_literal: true

require "set"

# Minimal `.env.local` loader to match the Next.js app's setup without extra gems.
# Format: KEY=VALUE (blank lines and `#` comments allowed).
preserved_keys = ENV.keys.to_set

[Rails.root.join(".env"), Rails.root.join(".env.local")].each do |env_path|
  next unless File.file?(env_path)

  File.foreach(env_path) do |line|
    line = line.strip
    next if line.empty? || line.start_with?("#")

    key, value = line.split("=", 2)
    next if key.nil? || value.nil?

    key = key.strip
    value = value.strip
    value = value[1..-2] if value.start_with?('"') && value.end_with?('"')

    next if preserved_keys.include?(key)

    ENV[key] = value
  end
end
