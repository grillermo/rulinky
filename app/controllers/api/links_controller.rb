# frozen_string_literal: true

require "securerandom"

module Api
  class LinksController < ApplicationController
    skip_before_action :verify_authenticity_token
    before_action :require_auth!, except: [:index, :job_status]

    def index
      links = Link.order(updated_at: :desc).select(:id, :url, :note, :read, :timestamp, :updated_at, :content)
      render json: links.map { |link| serialized_link(link) }
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

      id = SecureRandom.uuid
      timestamp_ms = (Time.now.to_f * 1000).to_i
      record = Link.create!(
        id: id,
        url: link,
        note: note,
        read: 0,
        timestamp: timestamp_ms,
        updated_at: timestamp_ms,
      )
      job_id = SecureRandom.uuid
      LinkContentJob.create!(id: job_id, link: record, status: "queued")
      FetchLinkContentJob.perform_async(job_id)

      render json: {
        message: "Link saved",
        id:,
        note:,
        job_id: job_id,
        title: record.display_title,
      }, status: :created
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

    def job_status
      job = LinkContentJob.includes(:link).find_by(id: params[:id])
      unless job
        render json: { error: "Job not found" }, status: :not_found
        return
      end

      render json: {
        id: job.id,
        status: job.status,
        finished: job.finished?,
        error: job.error_message,
        link: serialized_link(job.link),
      }
    rescue StandardError => e
      Rails.logger.error("Error loading job status: #{e.class}: #{e.message}")
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

    def serialized_link(link)
      {
        id: link.id,
        url: link.url,
        note: link.note,
        read: link.read,
        timestamp: link.timestamp,
        updated_at: link.updated_at,
        title: link.display_title,
        content_title: link.content_title,
      }
    end
  end
end
