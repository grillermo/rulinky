# frozen_string_literal: true

require "csv"
require "fileutils"
require "sqlite3"

root = File.expand_path("..", __dir__)
db_path = File.join(root, "data", "links.db")
out_path = ARGV[0] || File.join(root, "tmp", "links.csv")

unless File.file?(db_path)
  warn "SQLite DB not found: #{db_path}"
  exit 1
end

FileUtils.mkdir_p(File.dirname(out_path))

db = SQLite3::Database.new(db_path)
db.results_as_hash = true

columns = db.execute("PRAGMA table_info(links)").map { |row| row["name"] || row[1] }.compact
has_updated_at = columns.include?("updated_at")

select_sql =
  if has_updated_at
    "SELECT id, url, note, read, timestamp, updated_at FROM links ORDER BY COALESCE(updated_at, timestamp) DESC"
  else
    "SELECT id, url, note, read, timestamp, timestamp AS updated_at FROM links ORDER BY timestamp DESC"
  end

rows = db.execute(select_sql)

CSV.open(out_path, "w", write_headers: true, headers: %w[id url note read timestamp updated_at]) do |csv|
  rows.each do |row|
    csv << [
      row["id"],
      row["url"],
      row["note"],
      row["read"],
      row["timestamp"],
      row["updated_at"] || row["timestamp"],
    ]
  end
end

puts "Exported #{rows.length} links to #{out_path}"

