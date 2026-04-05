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

  def destroy
    link = Link.find_by(id: params[:id])
    link&.destroy
    
    redirect_to root_path
  end

  def unread
    link = Link.find_by(id: params[:id])
    if link
      link.update!(read: 0, updated_at: (Time.now.to_f * 1000).to_i)
    end
    
    redirect_back(fallback_location: root_path(filter: "read"))
  end
end
