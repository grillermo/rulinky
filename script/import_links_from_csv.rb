# frozen_string_literal: true

require "csv"

csv_path = ARGV[0]
unless csv_path && File.file?(csv_path)
  warn "Usage: bin/rails runner script/import_links_from_csv.rb /path/to/links.csv"
  exit 1
end

batch_size = (ENV["BATCH_SIZE"] || "1000").to_i
batch_size = 1000 if batch_size <= 0

total = 0
batch = []

flush = lambda do
  return if batch.empty?

  Link.upsert_all(batch, unique_by: :index_links_on_id)
  total += batch.length
  batch.clear
end

CSV.foreach(csv_path, headers: true) do |row|
  timestamp = row["timestamp"].to_i
  updated_at = row["updated_at"].to_i
  updated_at = timestamp if updated_at <= 0

  note = row["note"]
  note = nil if note && note.strip == ""

  batch << {
    id: row["id"],
    url: row["url"],
    note: note,
    read: row["read"].to_i,
    timestamp: timestamp,
    updated_at: updated_at,
  }

  flush.call if batch.length >= batch_size
end

flush.call

puts "Imported #{total} links from #{csv_path}"
