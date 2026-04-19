# frozen_string_literal: true

class Link < ApplicationRecord
  self.primary_key = "id"
  self.record_timestamps = false

  has_many :content_jobs, class_name: "LinkContentJob", dependent: :destroy

  validates :url, presence: true

  def content_title
    return nil if content.blank?

    Nokogiri::HTML(content).at("title")&.text&.squish.presence
  rescue StandardError
    nil
  end

  def display_title
    content_title.presence || note.to_s.strip.presence || url
  end

  def active_content_job
    content_jobs
      .select { |job| job.status.in?(%w[queued running]) }
      .max_by(&:created_at)
  end
end
