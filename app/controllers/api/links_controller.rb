# frozen_string_literal: true

require "net/http"
require "uri"
require "cgi"
require "securerandom"

module Api
  class LinksController < ApplicationController
    skip_before_action :verify_authenticity_token
    before_action :require_auth!, except: [:index]

    def index
      links = Link.order(updated_at: :desc).select(:id, :url, :note, :read, :timestamp, :updated_at)
      render json: links.as_json(only: %i[id url note read timestamp updated_at])
    rescue StandardError => e
      Rails.logger.error("Database error: #{e.class}: #{e.message}")
      render json: { error: "Internal Server Error" }, status: :internal_server_error
    end

    def create
      link = params[:link]
      note = params[:note]

      unless link.is_a?(String) && link.strip != ""
        render json: { error: "Invalid link" }, status: :bad_request
        return
      end

      note = note.to_s
      if note.strip == ""
        note = fetch_title(link) || ""
      end

      id = SecureRandom.uuid
      timestamp_ms = (Time.now.to_f * 1000).to_i
      Link.create!(
        id: id,
        url: link,
        note: note,
        read: 0,
        timestamp: timestamp_ms,
        updated_at: timestamp_ms,
      )

      render json: { message: "Link saved", id:, note: }, status: :created
    rescue StandardError => e
      Rails.logger.error("Error saving link: #{e.class}: #{e.message}")
      render json: { error: "Internal Server Error" }, status: :internal_server_error
    end

    def update
      id = params[:id]
      read = params[:read]

      unless id.is_a?(String) && id.strip != "" && (read == true || read == false)
        render json: { error: "Invalid payload" }, status: :bad_request
        return
      end

      updated_at_ms = (Time.now.to_f * 1000).to_i
      record = Link.find_by(id: id)
      unless record
        render json: { error: "Link not found" }, status: :not_found
        return
      end

      record.update!(read: (read ? 1 : 0), updated_at: updated_at_ms)
      render json: { message: "Link updated" }
    rescue StandardError => e
      Rails.logger.error("Error updating link: #{e.class}: #{e.message}")
      render json: { error: "Internal Server Error" }, status: :internal_server_error
    end

    def destroy
      id = params[:id]

      unless id.is_a?(String) && id.strip != ""
        render json: { error: "Invalid payload" }, status: :bad_request
        return
      end

      record = Link.find_by(id: id)
      unless record
        render json: { error: "Link not found" }, status: :not_found
        return
      end

      record.destroy!
      render json: { message: "Link deleted" }
    rescue StandardError => e
      Rails.logger.error("Error deleting link: #{e.class}: #{e.message}")
      render json: { error: "Internal Server Error" }, status: :internal_server_error
    end

    private

    def require_auth!
      token = request.headers["Authorization"].to_s
      expected = ENV["NEXT_PUBLIC_AUTH_TOKEN"].to_s
      expected = ENV["AUTH_TOKEN"].to_s if expected.empty?

      unless expected != "" && ActiveSupport::SecurityUtils.secure_compare(token, expected)
        render json: { error: "Unauthorized" }, status: :unauthorized
      end
    end

    def fetch_title(url)
      uri = URI.parse(url)
      return nil unless uri.is_a?(URI::HTTP) || uri.is_a?(URI::HTTPS)

      http = Net::HTTP.new(uri.host, uri.port)
      http.use_ssl = (uri.scheme == "https")
      http.open_timeout = 5
      http.read_timeout = 5

      req = Net::HTTP::Get.new(uri.request_uri, { "User-Agent" => "Mozilla/5.0 (compatible; Rulinky/1.0)" })
      res = http.request(req)
      return nil unless res.is_a?(Net::HTTPSuccess)

      html = res.body.to_s
      match = html.match(/<title[^>]*>([\s\S]*?)<\/title>/i)
      return nil unless match && match[1]

      CGI.unescapeHTML(match[1].strip)
    rescue StandardError => e
      Rails.logger.error("Error fetching title: #{e.class}: #{e.message}")
      nil
    end
  end
end
