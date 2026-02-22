# frozen_string_literal: true

class LinksController < ApplicationController
  def index
    @filter = params[:filter] == "read" ? "read" : "unread"

    scope = Link.order(updated_at: :desc).select(:id, :url, :note, :read, :timestamp, :updated_at)
    @links = scope.to_a

    @read_links_count = @links.count { |link| link.read.to_i == 1 }
    @unread_links_count = @links.count { |link| link.read.to_i == 0 }

    @filtered_links =
      if @filter == "read"
        @links.select { |link| link.read.to_i == 1 }
      else
        @links.select { |link| link.read.to_i == 0 }
      end
  end
end
