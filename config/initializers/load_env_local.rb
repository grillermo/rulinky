# frozen_string_literal: true

# Minimal `.env.local` loader to match the Next.js app's setup without extra gems.
# Format: KEY=VALUE (blank lines and `#` comments allowed).
env_path = Rails.root.join(".env.local")
if File.file?(env_path)
  File.foreach(env_path) do |line|
    line = line.strip
    next if line.empty? || line.start_with?("#")

    key, value = line.split("=", 2)
    next if key.nil? || value.nil?

    key = key.strip
    value = value.strip
    value = value[1..-2] if value.start_with?('"') && value.end_with?('"')

    ENV[key] ||= value
  end
end

