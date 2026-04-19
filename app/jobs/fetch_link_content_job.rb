# frozen_string_literal: true

require "net/http"
require "json"
require "cgi"

class FetchLinkContentJob
  include SuckerPunch::Job

  TWITTER_HOSTS = %w[x.com www.x.com mobile.x.com twitter.com www.twitter.com mobile.twitter.com].freeze
  APIFY_TWITTER_ACTOR_ID = "apidojo~twitter-scraper-lite"

  def perform(job_id)
    job = LinkContentJob.find_by(id: job_id)
    return unless job

    job.update!(status: "running", error_message: nil)

    link = job.link
    html = fetch_html(link.url)
    link.update!(content: html)

    job.update!(status: "finished")
  rescue StandardError => e
    Rails.logger.error("FetchLinkContentJob failed for #{job_id}: #{e.class}: #{e.message}")
    job&.update(status: "failed", error_message: e.message)
  end

  private

  def fetch_html(url)
    return fetch_twitter_html(url) if twitter_url?(url)

    fetch_firecrawl_html(url)
  end

  def twitter_url?(url)
    uri = URI.parse(url)
    TWITTER_HOSTS.include?(uri.host.to_s.downcase)
  rescue URI::InvalidURIError
    false
  end

  def fetch_twitter_html(url)
    api_key = ENV["APIFY_API_KEY"].to_s
    raise "APIFY_API_KEY is missing" if api_key.empty?

    uri = URI("https://api.apify.com/v2/acts/#{APIFY_TWITTER_ACTOR_ID}/run-sync-get-dataset-items?clean=true&format=json")
    http = Net::HTTP.new(uri.host, uri.port)
    http.use_ssl = true
    http.open_timeout = 10
    http.read_timeout = 120

    request = Net::HTTP::Post.new(
      uri.request_uri,
      {
        "Authorization" => "Bearer #{api_key}",
        "Content-Type" => "application/json",
      },
    )
    request.body = {
      startUrls: [url],
      maxItems: 1,
    }.to_json

    response = http.request(request)
    payload = JSON.parse(response.body)

    unless response.is_a?(Net::HTTPSuccess) && payload.is_a?(Array)
      message = payload.dig("error", "message") if payload.is_a?(Hash)
      raise(message.presence || "Apify Twitter scrape failed")
    end

    item = payload.first
    raise "Apify returned no Twitter content" unless item.is_a?(Hash)

    build_twitter_html(item, url)
  rescue JSON::ParserError => e
    raise "Apify returned invalid JSON: #{e.message}"
  end

  def build_twitter_html(item, fallback_url)
    author_name = pick_first_present(item, "authorName", "name", "userName")
    author_handle = pick_first_present(item, "authorUserName", "userScreenName", "twitterHandle", "userName")
    text = pick_first_present(item, "text", "fullText")
    url = pick_first_present(item, "url", "twitterUrl") || fallback_url
    posted_at = pick_first_present(item, "createdAt", "created_at")
    quote_count = pick_first_present(item, "quoteCount", "quotes")
    reply_count = pick_first_present(item, "replyCount", "replies")
    like_count = pick_first_present(item, "likeCount", "likes")
    retweet_count = pick_first_present(item, "retweetCount", "retweets")
    bookmark_count = pick_first_present(item, "bookmarkCount", "bookmarks")
    media_urls = Array(item["extendedEntities"] || item["media"] || item["photos"] || item["images"]).filter_map do |entry|
      case entry
      when String
        entry
      when Hash
        pick_first_present(entry, "media_url_https", "media_url", "url")
      end
    end

    title_parts = []
    title_parts << author_name if author_name.present?
    title_parts << "@#{author_handle}" if author_handle.present?
    title_parts << "on X"
    title = title_parts.join(" ")

    metadata = [
      ["URL", url],
      ["Posted", posted_at],
      ["Replies", reply_count],
      ["Retweets", retweet_count],
      ["Quotes", quote_count],
      ["Likes", like_count],
      ["Bookmarks", bookmark_count],
    ].select { |_, value| value.present? }

    body_parts = []
    body_parts << "<h1>#{escape_html(title)}</h1>"
    body_parts << "<p>#{escape_html(text)}</p>" if text.present?

    if metadata.any?
      body_parts << "<ul>#{metadata.map { |label, value| "<li><strong>#{escape_html(label)}:</strong> #{escape_html(value.to_s)}</li>" }.join}</ul>"
    end

    if media_urls.any?
      media_links = media_urls.uniq.map do |media_url|
        escaped_url = escape_html(media_url)
        %(<li><a href="#{escaped_url}">#{escaped_url}</a></li>)
      end.join
      body_parts << "<h2>Media</h2><ul>#{media_links}</ul>"
    end

    body_parts << "<h2>Raw Data</h2><pre>#{escape_html(JSON.pretty_generate(item))}</pre>"

    <<~HTML
      <!DOCTYPE html>
      <html lang="en">
        <head>
          <meta charset="utf-8">
          <title>#{escape_html(title)}</title>
        </head>
        <body>
          #{body_parts.join("\n")}
        </body>
      </html>
    HTML
  end

  def pick_first_present(hash, *keys)
    keys.each do |key|
      value = hash[key]
      return value if value.present?
    end

    nil
  end

  def escape_html(value)
    CGI.escapeHTML(value.to_s)
  end

  def fetch_firecrawl_html(url)
    api_key = ENV["FIRECRAWL_API_KEY"].to_s
    raise "FIRECRAWL_API_KEY is missing" if api_key.empty?

    uri = URI("https://api.firecrawl.dev/v2/scrape")
    http = Net::HTTP.new(uri.host, uri.port)
    http.use_ssl = true
    http.open_timeout = 10
    http.read_timeout = 90

    request = Net::HTTP::Post.new(
      uri.request_uri,
      {
        "Authorization" => "Bearer #{api_key}",
        "Content-Type" => "application/json",
      },
    )
    request.body = {
      url: url,
      formats: ["html"],
    }.to_json

    response = http.request(request)
    payload = JSON.parse(response.body)

    unless response.is_a?(Net::HTTPSuccess) && payload["success"] == true
      message = payload["error"].presence || payload["message"].presence || "Firecrawl request failed"
      raise message
    end

    html = payload.dig("data", "html").to_s
    raise "Firecrawl returned empty HTML content" if html.blank?

    html
  rescue JSON::ParserError => e
    raise "Firecrawl returned invalid JSON: #{e.message}"
  end
end
