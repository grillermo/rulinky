# frozen_string_literal: true

require "net/http"
require "uri"
require "json"

# Posts crawled HTML to readitsoon's /preview endpoint and extracts the clean
# article title and sanitized HTML from the returned preview page.
class ReaditsoonPreview
  PREVIEW_URL = "https://readitsoon.app/preview"
  DEFAULT_EMAIL = "guillermo.siliceo@kindle.com"

  def self.call(html, url)
    new(html, url).call
  end

  def initialize(html, url)
    @html = html
    @url = url
  end

  def call
    response = post
    raise(error_message(response)) unless response.is_a?(Net::HTTPSuccess)

    parse(response.body)
  end

  private

  def post
    uri = URI(PREVIEW_URL)
    http = Net::HTTP.new(uri.host, uri.port)
    http.use_ssl = true
    http.open_timeout = 10
    http.read_timeout = 90

    request = Net::HTTP::Post.new(uri.request_uri)
    request.set_form_data(
      "inputhtml" => @html,
      "url" => @url,
      "email" => email,
    )
    http.request(request)
  end

  def parse(body)
    doc = Nokogiri::HTML(body)
    form = doc.at_css('form[x-data^="previewSendForm"]')
    raise "readitsoon preview form not found in response" unless form

    title = form.at_css('input[name="title"]')&.[]("value").to_s
    content = doc.at_css('input[name="html_content"]')&.[]("value").to_s

    { title: title, content: content }
  end

  def error_message(response)
    payload = JSON.parse(response.body)
    message = payload["error"].presence || payload["errors"]&.join(", ").presence if payload.is_a?(Hash)
    message.presence || "readitsoon preview failed (#{response.code})"
  rescue JSON::ParserError
    "readitsoon preview failed (#{response.code})"
  end

  def email
    ENV["READITSOON_EMAIL"].presence || DEFAULT_EMAIL
  end
end
