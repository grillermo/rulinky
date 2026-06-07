# frozen_string_literal: true

class Link < ApplicationRecord
  self.primary_key = "id"
  self.record_timestamps = false

  has_many :content_jobs, class_name: "LinkContentJob", dependent: :destroy
  belongs_to :user

  validates :url, presence: true

  def active_content_job
    content_jobs
      .select { |job| job.status.in?(%w[queued running]) }
      .max_by(&:created_at)
  end
end
