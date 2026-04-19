# frozen_string_literal: true

require "securerandom"
require "set"
require "uri"

scope = Link.where(read: 0).order(updated_at: :desc)
scope = scope.where(content: [nil, ""]) unless ENV["INCLUDE_WITH_CONTENT"] == "1"

created = 0
skipped_active = 0
skipped_finished = 0
skipped_non_twitter = 0

twitter_hosts = FetchLinkContentJob::TWITTER_HOSTS.to_set

scope.find_each do |link|
  begin
    host = URI.parse(link.url).host.to_s.downcase
  rescue URI::InvalidURIError
    host = ""
  end

  unless twitter_hosts.include?(host)
    skipped_non_twitter += 1
    next
  end

  if link.content_jobs.where(status: %w[queued running]).exists?
    skipped_active += 1
    next
  end

  if ENV["RETRY_FINISHED"] != "1" && link.content_jobs.where(status: "finished").exists?
    skipped_finished += 1
    next
  end

  job_id = SecureRandom.uuid
  LinkContentJob.create!(id: job_id, link: link, status: "queued")
  FetchLinkContentJob.perform_async(job_id)
  created += 1
end

puts "Queued #{created} content jobs for unread Twitter/X links"
puts "Skipped #{skipped_non_twitter} unread non-Twitter/X links"
puts "Skipped #{skipped_active} unread Twitter/X links with an active content job"
puts "Skipped #{skipped_finished} unread Twitter/X links with a finished content job"
