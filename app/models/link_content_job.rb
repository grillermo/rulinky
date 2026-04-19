# frozen_string_literal: true

class LinkContentJob < ApplicationRecord
  self.primary_key = "id"

  belongs_to :link

  validates :status, presence: true

  def finished?
    status.in?(%w[finished failed])
  end
end
