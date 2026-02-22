# frozen_string_literal: true

class Link < ApplicationRecord
  self.primary_key = "id"
  self.record_timestamps = false

  validates :url, presence: true
end

