# frozen_string_literal: true

class LinksController < ApplicationController
  def index
    @filter = params[:filter] == "read" ? "read" : "unread"

    scope = Link.includes(:content_jobs).order(updated_at: :desc).select(:id, :url, :note, :read, :timestamp, :updated_at, :content)
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

    if request.xhr?
      head :ok
    else
      redirect_to root_path
    end
  end

  def read
    update_read_status(1)
  end

  def unread
    update_read_status(0)
  end

  private

  def update_read_status(read_value)
    link = Link.find_by(id: params[:id])
    link&.update!(read: read_value, updated_at: (Time.now.to_f * 1000).to_i)

    if request.xhr?
      head :ok
    else
      redirect_back(fallback_location: root_path(filter: read_value == 1 ? "unread" : "read"))
    end
  end
end
